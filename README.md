<div align="center">

<img src="web/src/portal/public/favicon.svg" width="72" alt="">

# Gozar

**A smart DNS gateway with a real product around it.**
Customers change one DNS setting; sanctioned services start working; everything else stays direct.

*[فارسی](README.fa.md)*

</div>

<p align="center">
  <img src="docs/screenshots/admin-dashboard.png" width="68%" alt="Admin dashboard">
  <img src="docs/screenshots/portal-home.png" width="22%" alt="Customer portal">
</p>

---

## How it works

```
 customer device            gate (inside the country)                 hub (abroad)            service
 ┌──────────────┐  DNS     ┌──────────────────────────┐              ┌──────────────────┐    ┌─────────┐
 │ PS5, phone,  │ ───────► │ built-in resolver        │              │ egress           │    │ Sony    │
 │ laptop       │          │  routed name? answer     │   TCP, TLS   │  reads the name  │    │ Steam   │
 │              │  TLS     │  with the gate's address │   untouched  │  and connects    │ ─► │ OpenAI  │
 │              │ ───────► │ proxy: reads SNI, checks │ ───────────► │                  │    │ …       │
 └──────────────┘          │ the plan, counts bytes,  │              │ database, admin  │    └─────────┘
                           │ paces speed              │ ◄── sync ──  │ console, API     │
                           │ customer portal          │   every 30s  │                  │
                           └──────────────────────────┘              └──────────────────┘
```

* The **gate** answers DNS for its customers. A name their plan routes resolves to the gate itself; everything else resolves normally and never touches Gozar.
* A routed connection is carried to the **hub** as raw TCP. TLS is never terminated or decrypted — the gate only reads the hostname a client announces.
* The hub keeps one database for every gate, so a customer's allowance means the same thing on every gate. Gates always dial the hub, never the other way round.

Everything — DNS server, proxy, speed limiting, usage accounting, STUN, certificates, both web apps — is **one Go binary**. No nginx, no dnsmasq, no nftables rules, no containers, no pip or npm on the server.

## What you get

| | |
|---|---|
| **Customer portal** | Sign-up, one-tap device registration, usage ring and daily chart, register this connection in one tap or another network's address by hand (a console at home), per-device setup guides (PS5, Xbox, Switch, Android, iPhone, Windows, macOS, router), a live *“is my DNS set up?”* test |
| **Admin console** | Live dashboard, customers with search / filters / Ctrl-K palette, plan editor with a visual service picker, receipt review, gate health, activity log, backup and restore |
| **Payments** | Online through IDPay — verified from the gate inside Iran, plan renewed the moment the bank confirms — or card transfer with a photo of the receipt for you to approve |
| **Plans** | Price, data quota, duration, speed cap, device limit, and exactly which services are routed — down to single domains |
| **Accounting** | Per-customer byte counting in the proxy itself, idempotent sync, daily history, automatic expiry and period reset |
| **Speed caps** | Paced, not dropped: capped downloads run smoothly, just slower |
| **Security** | Argon2id passwords, hashed session tokens, TOTP two-step sign-in for admins, throttled logins, pinned hub certificate, no open proxy |
| **Languages** | Persian (RTL, Solar Hijri dates, Persian digits) and English, light and dark themes, self-hosted fonts |
| **Certificates** | Let's Encrypt via TLS-ALPN on port 443 — port 80 is never borrowed, so console downloads never pause for a renewal |

## Screenshots

| | |
|---|---|
| ![Customers](docs/screenshots/admin-users.png) | ![Plan editor](docs/screenshots/admin-plan-editor.png) |
| ![Receipts](docs/screenshots/admin-receipts.png) | ![Plans in the portal](docs/screenshots/portal-plans.png) |

<p align="center">
  <img src="docs/screenshots/portal-home-light.png" width="24%" alt="">
  <img src="docs/screenshots/portal-setup.png" width="24%" alt="">
</p>

## Install

You need two Linux servers (Debian/Ubuntu or anything with systemd), each with a public IPv4 address:

* **hub** — outside the filtered network
* **gate** — inside the country; customers use its address as DNS

**1. On the hub:**

```bash
curl -fsSL https://raw.githubusercontent.com/amor7/gozarDNS/main/install.sh | sudo bash
```

Choose **Hub**. The wizard asks for an optional domain for the console, then prints the console address, the admin password (once) and a **join token** for your first gate.

**2. On the gate:** run the same command, choose **Gate** and paste the join token. The wizard checks it can reach the hub, frees port 53 from `systemd-resolved` if needed, and starts.

That's it. Open the console, adjust the two starter plans, and send customers the portal address.

### Ports

| | gate | hub |
|---|---|---|
| **53** udp+tcp | DNS for customers | — |
| **80, 443** tcp | proxied traffic, customer portal (by domain) | egress, answering gates only; console (by domain) |
| **8443** tcp | customer portal by IP | — |
| **3478** udp | STUN for console NAT detection | — |
| **7443** tcp | — | gate sync API |
| random tcp | — | admin console (chosen at setup) |

SSH is never touched. Gozar does not manage your firewall — open these ports at your provider.

### A new gate starts open

A fresh gate answers everyone with the default plan, so you can test it before any customer exists. It locks itself to registered addresses the moment the first customer registers. Change this per gate in the console (*Gates → Access*).

## Everyday commands

```bash
sudo gozar status              # what this machine is doing
sudo gozar logs -f             # follow the logs (-e: warnings and errors only)
sudo gozar watch sara          # gate: live DNS lookups of one customer
sudo gozar check gemini.google.com sara   # gate: what happens to this name
sudo gozar admin password      # hub: new admin password
sudo gozar admin 2fa-off admin # hub: lost your phone
sudo gozar gate add shiraz-1   # hub: join token for another gate
sudo gozar backup              # hub: full database backup
sudo gozar update              # latest release, checksum-verified
```

`gozar watch` is the tool for finding what an app needs: open it on the device until it fails, and see which names went `direct`, which were `routed`, and which are `filtered` inside the country. Add the missing ones in *Domains → Custom domains*.

## Choosing servers

The network path between gate and hub matters more than anything in the code. Some foreign ranges are throttled or cut off from inside the country no matter what runs on them. Rent both machines by the hour first, test from a few different customer connections (home line and mobile data behave differently), then commit.

## Development

```bash
make web                     # build both web apps into the binary
make build                   # ./bin/gozar
make test

go run ./tools/devstack      # hub + gate on localhost with demo data
npm --prefix web run dev:admin    # http://localhost:5173  admin / admin12345
npm --prefix web run dev:portal   # http://localhost:5174  sara / sara1234
node tools/screenshots/shoot.mjs  # regenerate docs/screenshots
```

| | |
|---|---|
| `cmd/gozar` | the single binary |
| `internal/hub` | admin API, customer API, gate sync, egress proxy |
| `internal/gate` | resolver wiring, proxy, portal, sync client |
| `internal/dnsd` | the DNS server, live watch, reachability pinning |
| `internal/store` | SQLite (pure Go, no cgo) |
| `internal/catalog` | the built-in service and domain catalogue |
| `web/src/admin`, `web/src/portal` | React + TypeScript + Tailwind apps sharing `web/src/shared` |

## Known limits

* Upload is not capped, only download.
* Traffic is carried twice (gate and hub): a customer's 1 GB costs about 2 GB on each server.
* Services that don't use HTTP(S), or that resolve names without system DNS, can't be routed by DNS.

## License

MIT
