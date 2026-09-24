export type Text = { fa: string; en: string };
export type State = "active" | "no_plan" | "expired" | "over_quota" | "disabled";

export type Admin = { id: number; username: string; totp_enabled: boolean; created_at: number; last_login_at: number | null };

export type User = {
  id: number;
  username: string;
  display_name: string;
  phone: string;
  plan_id: number | null;
  plan_name: string;
  enabled: boolean;
  quota_bytes: number;
  used_bytes: number;
  period_start: number | null;
  expires_at: number | null;
  speed_kbps: number;
  max_devices: number;
  note: string;
  created_at: number;
  last_seen_at: number | null;
  devices: number;
  state: State;
  has_password: boolean;
};

export type Device = { id: number; user_id: number; ip: string; label: string; added_at: number; last_seen_at: number | null };

export type Plan = {
  id: number;
  name: string;
  description: string;
  price: number;
  quota_bytes: number;
  duration_days: number;
  renewal: "expire" | "reset";
  speed_kbps: number;
  max_devices: number;
  groups: string[];
  excluded: string[];
  public: boolean;
  is_default: boolean;
  color: string;
  sort: number;
  created_at: number;
  users: number;
  domains?: number;
};

export type Payment = {
  id: number;
  user_id: number;
  username: string;
  plan_id: number | null;
  plan_name: string;
  amount: number;
  note: string;
  has_receipt: boolean;
  receipt_type: string;
  status: "pending" | "approved" | "rejected" | "failed";
  method: "receipt" | "idpay" | "zarinpal" | "payping";
  track_id: string;
  admin_note: string;
  created_at: number;
  decided_at: number | null;
  decided_by: string;
};

export type Sample = { cpu: number; load1: number; mem_used: number; mem_total: number; disk_used: number; disk_total: number; rx_bps: number; tx_bps: number; uptime: number; conns: number };

export type Gate = {
  id: string;
  name: string;
  public_ip: string;
  seen_ip: string;
  version: string;
  enforce: "auto" | "on" | "off";
  tunnel: "on" | "off";
  last_seen_at: number | null;
  stats: { host?: Sample; stats?: { queries: number; routed: number; refused: number; open_conns: number; tunnel?: "" | "up" | "down" } };
  created_at: number;
  online: boolean;
};

export type Activity = { id: number; at: number; actor: string; kind: string; detail: string };

export type Totals = {
  users: number;
  active: number;
  devices: number;
  online_now: number;
  traffic_today: number;
  traffic_month: number;
  pending_payments: number;
  revenue_month: number;
  expiring_soon: number;
};

export type Overview = {
  totals: Totals;
  traffic: { ts: number; up: number; down: number }[];
  daily: { day: string; bytes: number }[];
  top: { id: number; username: string; bytes: number }[];
  gates: Gate[];
  activity: Activity[];
  expiring: User[];
  hub: { sample: Sample; ipv6: boolean; conns: number };
};

export type Group = { id: string; name: Text; heavy: boolean; domains: string[] };
export type Service = { id: string; name: Text; category: string; color: string; groups: Group[] };
export type Rule = { id: string; action: "direct" | "pin"; title: Text; why: Text; domains: string[]; enabled: boolean };
export type CustomDomain = { domain: string; action: "route" | "direct"; note: string; added_at: number };
export type Catalog = {
  categories: { id: string; name: Text }[];
  services: Service[];
  rules: Rule[];
  custom: CustomDomain[];
  custom_key: string;
};
