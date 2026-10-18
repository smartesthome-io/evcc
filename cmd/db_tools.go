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
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}

	return nil
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

	env := []string{
		"PGHOST=" + cfg.Host,
		"PGPORT=" + strconv.Itoa(int(cfg.Port)),
		"PGUSER=" + cfg.User,
		"PGDATABASE=" + cfg.Database,
	}

	if cfg.Password != "" {
		env = append(env, "PGPASSWORD="+cfg.Password)
	}

	settings := dsnSettings(dsn)
	for key, variable := range map[string]string{
		"sslmode":     "PGSSLMODE",
		"sslrootcert": "PGSSLROOTCERT",
		"sslcert":     "PGSSLCERT",
		"sslkey":      "PGSSLKEY",
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

	for _, field := range strings.Fields(dsn) {
		if k, v, ok := strings.Cut(field, "="); ok {
			res[k] = strings.Trim(v, "'")
		}
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
