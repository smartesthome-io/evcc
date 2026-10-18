// the subset of the evcc state this ui reads, see GET /api/state

export interface Meter {
  name?: string;
  title?: string;
  icon?: string;
  power: number;
}

export interface Battery {
  /** W, positive = discharging, negative = charging */
  power: number;
  soc: number;
  capacity: number;
}

export interface Loadpoint {
  title: string;
  mode: "off" | "smart" | "now";
  connected: boolean;
  charging: boolean;
  enabled: boolean;
  chargePower: number;
  chargedEnergy: number;
  sessionEnergy: number;
  vehicleName: string;
  vehicleTitle: string;
  vehicleSoc: number;
  vehicleRange: number;
  limitSoc: number;
  chargeRemainingDuration?: number;
  chargerFeatureHeating: boolean;
  phasesActive: number;
}

export interface State {
  siteTitle?: string;
  startupCompleted?: boolean;
  grid?: Meter;
  pv?: Meter[];
  pvPower?: number;
  battery?: Battery;
  batteryMode?: string;
  homePower?: number;
  aux?: Meter[];
  ext?: Meter[];
  consumers?: Meter[];
  loadpoints: Loadpoint[];
  version?: string;
}
