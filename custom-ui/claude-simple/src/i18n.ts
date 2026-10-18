const en = {
  "tab.now": "Now",
  "tab.today": "Today",
  "tab.config": "Config",
  "status.charging-car": "Charging car",
  "status.discharging-battery": "Discharging home battery",
  "status.charging-battery": "Charging home battery",
  "status.grid-export": "Grid export",
  "status.grid-import": "Grid import",
  "status.idle": "Idle",
  "flow.title": "Energy flow",
  "flow.empty": "No energy flow right now",
  "flow.pv": "Solar",
  "flow.gridImport": "Grid",
  "flow.batteryDischarge": "Battery",
  "flow.home": "Home",
  "flow.consumer": "Consumer",
  "flow.batteryCharge": "Battery",
  "flow.loadpoint": "Charge point",
  "flow.gridExport": "Export",
  "battery.title": "Home battery",
  "battery.charging": "Charging",
  "battery.discharging": "Discharging",
  "battery.standby": "Standby",
  "loadpoints.title": "Charging",
  "loadpoints.disconnected": "No vehicle connected",
  "loadpoints.connected": "Connected",
  "loadpoints.charging": "Charging",
  "loadpoints.noVehicle": "Vehicle without data",
  "loadpoints.remaining": "remaining",
  "mode.off": "Off",
  "mode.smart": "Smart",
  "mode.now": "Fast",
  "heating.title": "Heating",
  "heating.running": "Running",
  "heating.standby": "Standby",
  "consumers.title": "Other consumers",
  offline: "Connection lost, retrying",
  "soon.title": "Coming next",
  "config.classic": "Open the full configuration",
  "config.hint": "Device, tariff and system settings are still managed in the classic interface.",
  classic: "Classic interface",
} as const;

const de: Record<keyof typeof en, string> = {
  "tab.now": "Jetzt",
  "tab.today": "Heute",
  "tab.config": "Einstellungen",
  "status.charging-car": "Auto lädt",
  "status.discharging-battery": "Hausbatterie entlädt",
  "status.charging-battery": "Hausbatterie lädt",
  "status.grid-export": "Netzeinspeisung",
  "status.grid-import": "Netzbezug",
  "status.idle": "Ruhe",
  "flow.title": "Energiefluss",
  "flow.empty": "Gerade kein Energiefluss",
  "flow.pv": "Solar",
  "flow.gridImport": "Netz",
  "flow.batteryDischarge": "Batterie",
  "flow.home": "Haus",
  "flow.consumer": "Verbraucher",
  "flow.batteryCharge": "Batterie",
  "flow.loadpoint": "Ladepunkt",
  "flow.gridExport": "Einspeisung",
  "battery.title": "Hausbatterie",
  "battery.charging": "Lädt",
  "battery.discharging": "Entlädt",
  "battery.standby": "Bereit",
  "loadpoints.title": "Laden",
  "loadpoints.disconnected": "Kein Fahrzeug verbunden",
  "loadpoints.connected": "Verbunden",
  "loadpoints.charging": "Lädt",
  "loadpoints.noVehicle": "Fahrzeug ohne Daten",
  "loadpoints.remaining": "verbleibend",
  "mode.off": "Aus",
  "mode.smart": "Smart",
  "mode.now": "Schnell",
  "heating.title": "Heizung",
  "heating.running": "Läuft",
  "heating.standby": "Bereit",
  "consumers.title": "Weitere Verbraucher",
  offline: "Verbindung verloren, neuer Versuch",
  "soon.title": "Kommt als Nächstes",
  "config.classic": "Vollständige Konfiguration öffnen",
  "config.hint":
    "Geräte, Tarife und Systemeinstellungen werden vorerst in der klassischen Oberfläche verwaltet.",
  classic: "Klassische Oberfläche",
};

export type Key = keyof typeof en;

const dict: Record<string, Record<Key, string>> = { en, de };

const lang = (navigator.language || "en").slice(0, 2);

export function t(key: Key): string {
  return (dict[lang] ?? en)[key];
}

export function labels(prefix: string, keys: string[]): Record<string, string> {
  return Object.fromEntries(keys.map((k) => [k, t(`${prefix}.${k}` as Key)]));
}
