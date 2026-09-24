export type Text = { fa: string; en: string };
export type ServiceBrief = { id: string; name: Text; color: string; heavy: boolean };

export type PublicPlan = {
  id: number;
  name: string;
  description: string;
  price: number;
  quota_bytes: number;
  duration_days: number;
  renewal: "expire" | "reset";
  speed_kbps: number;
  max_devices: number;
  color: string;
  services: ServiceBrief[];
};

export type Info = {
  brand: string;
  support: string;
  notice: string;
  currency: string;
  payment_info: string;
  signup_open: boolean;
  online_pay: boolean;
  dns: string;
  your_ip: string;
  plans: PublicPlan[];
};

export type State = "active" | "no_plan" | "expired" | "over_quota" | "disabled";

export type Device = { id: number; ip: string; label: string; added_at: number; last_seen_at: number | null };

export type Payment = {
  id: number;
  plan_name: string;
  amount: number;
  note: string;
  status: "pending" | "approved" | "rejected" | "failed";
  method: "receipt" | "idpay";
  track_id: string;
  admin_note: string;
  created_at: number;
};

export type Me = {
  user: {
    id: number;
    username: string;
    display_name: string;
    phone: string;
    state: State;
    quota_bytes: number;
    used_bytes: number;
    period_start: number | null;
    expires_at: number | null;
    speed_kbps: number;
    max_devices: number;
    created_at: number;
  };
  plan: PublicPlan | null;
  devices: Device[];
  current_ip: string;
  ipv6: boolean;
  this_device: number | null;
  ip_taken: boolean;
  usage: { day: string; bytes: number }[];
  payments: Payment[];
  dns: string;
};
