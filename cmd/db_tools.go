package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// maxToolOutput limits the tool output that is added to error messages
const maxToolOutput = 2048

// toolRunner runs a PostgreSQL client tool, replaced in tests
var toolRunner = runTool

func runTool(ctx context.Context, name string, args, env []string, stdin io.Reader, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout

	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: maxToolOutput}

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%s not found, install the PostgreSQL client tools (the version must not be older than the server)", name)
		}
		if msg := toolMessages(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}

	return nil
}

// toolMessages keeps the error lines of the PostgreSQL tools. Detail and command lines are dropped,
// they can contain the data of the failing statement.
func toolMessages(stderr string) string {
	var lines []string
	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.HasPrefix(line, "pg_dump:") || strings.HasPrefix(line, "pg_restore:") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return strings.Join(lines, "; ")
}

type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// pgEnv returns the environment that connects the PostgreSQL client tools to the dsn.
// Credentials are passed this way so they do not show up in the process list.
// It also returns the database name, pg_restore needs it as an argument.
func pgEnv(dsn string) ([]string, string, error) {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return nil, "", fmt.Errorf("invalid postgres dsn: %w", err)
	}

	settings := dsnSettings(dsn)
	if settings["sslpassword"] != "" {
		return nil, "", errors.New("sslpassword is not supported for backup and restore, use an unencrypted client key")
	}

	// libpq accepts several hosts, the fallbacks of the parsed config also contain tls variants of the same host
	var hosts, ports []string
	seen := make(map[string]bool)
	for _, c := range append([]*pgconn.FallbackConfig{{Host: cfg.Host, Port: cfg.Port}}, cfg.Fallbacks...) {
		port := strconv.Itoa(int(c.Port))
		if key := c.Host + ":" + port; !seen[key] {
			seen[key] = true
			hosts, ports = append(hosts, c.Host), append(ports, port)
		}
	}

	env := []string{
		"PGHOST=" + strings.Join(hosts, ","),
		"PGPORT=" + strings.Join(ports, ","),
		"PGUSER=" + cfg.User,
		"PGDATABASE=" + cfg.Database,
	}

	if cfg.Password != "" {
		env = append(env, "PGPASSWORD="+cfg.Password)
	}

	for key, variable := range map[string]string{
		"sslmode":              "PGSSLMODE",
		"sslrootcert":          "PGSSLROOTCERT",
		"sslcert":              "PGSSLCERT",
		"sslkey":               "PGSSLKEY",
		"sslcrl":               "PGSSLCRL",
		"connect_timeout":      "PGCONNECT_TIMEOUT",
		"target_session_attrs": "PGTARGETSESSIONATTRS",
		"krbsrvname":           "PGKRBSRVNAME",
		"service":              "PGSERVICE",
		"passfile":             "PGPASSFILE",
	} {
		if v := settings[key]; v != "" {
			env = append(env, variable+"="+v)
		}
	}

	return env, cfg.Database, nil
}

// dsnSettings returns the parameters of a url or key value dsn
func dsnSettings(dsn string) map[string]string {
	res := make(map[string]string)

	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		for k, v := range u.Query() {
			res[k] = v[0]
		}
		return res
	}

	// key value form: key = value, values may be single quoted with backslash escapes
	rest := strings.TrimSpace(dsn)
	for rest != "" {
		key, after, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}

		key = strings.TrimSpace(key)
		after = strings.TrimLeft(after, " \t")

		var value string
		if strings.HasPrefix(after, "'") {
			var b strings.Builder
			i := 1
			for ; i < len(after) && after[i] != '\''; i++ {
				if after[i] == '\\' && i+1 < len(after) {
					i++
				}
				b.WriteByte(after[i])
			}
			value, rest = b.String(), strings.TrimSpace(after[min(i+1, len(after)):])
		} else {
			value, rest, _ = strings.Cut(after, " ")
			rest = strings.TrimSpace(rest)
		}

		res[key] = value
	}

	return res
}

// confirm asks for confirmation unless yes is set
func confirm(question string, yes bool, in *os.File) error {
	if yes {
		return nil
	}

	if !term.IsTerminal(int(in.Fd())) {
		return errors.New("confirmation required, use --yes to continue without asking")
	}

	fmt.Fprintf(os.Stderr, "%s [y/N]: ", question)

	var answer string
	_, _ = fmt.Fscanln(in, &answer)

	if answer = strings.ToLower(answer); answer != "y" && answer != "yes" {
		return errors.New("aborted")
	}

	return nil
}

// databaseFromConfig returns type and dsn of the configured database
func databaseFromConfig(cmd *cobra.Command) (string, string, error) {
	if err := loadConfigFile(&conf, !cmd.Flag(flagIgnoreDatabase).Changed); err != nil {
		return "", "", err
	}

	return databaseTarget(conf.Database)
}
