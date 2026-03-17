# WireGuard Site-to-Site for Proxmaster

Ziel: nur ein offener Ingress-Port (UDP), API/SSH ausschliesslich ueber Tunnel.

## 1) Schluessel erzeugen (beide Seiten)

```bash
umask 077
wg genkey | tee privatekey | wg pubkey > publickey
```

## 2) Beispiel-Konfiguration Proxmaster-VM (`/etc/wireguard/wg0.conf`)

```ini
[Interface]
Address = 10.13.13.2/24
ListenPort = 51820
PrivateKey = <VM_PRIVATE_KEY>

[Peer]
PublicKey = <PC_PUBLIC_KEY>
AllowedIPs = 10.13.13.1/32
PersistentKeepalive = 25
```

## 3) Beispiel-Konfiguration lokaler PC (`wg0.conf`)

```ini
[Interface]
Address = 10.13.13.1/24
PrivateKey = <PC_PRIVATE_KEY>

[Peer]
PublicKey = <VM_PUBLIC_KEY>
Endpoint = <PUBLIC_IP_OR_DNS_OF_REMOTE>:51820
AllowedIPs = 10.13.13.2/32,10.0.0.0/16
PersistentKeepalive = 25
```

## 4) Firewall-Regeln auf VM

- Erlauben: `udp/51820` aus WAN.
- Erlauben: `tcp/8080` und `tcp/22` nur aus `10.13.13.0/24`.
- Verbieten: `tcp/8080` und `tcp/22` aus WAN.

UFW-Beispiel:

```bash
ufw allow 51820/udp
ufw allow from 10.13.13.0/24 to any port 8080 proto tcp
ufw allow from 10.13.13.0/24 to any port 22 proto tcp
ufw deny 8080/tcp
ufw deny 22/tcp
```

## 5) Dienst aktivieren

```bash
systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0
wg show
```

## 6) Proxmaster `.env`

```env
PROXMASTER_WIREGUARD_INTERFACE=wg0
PROXMASTER_BREAKGLASS_ENABLE_CMD=/opt/proxmaster/infra/ops/breakglass/ssh-breakglass-enable.sh 22 10.13.13.0/24
PROXMASTER_BREAKGLASS_DISABLE_CMD=/opt/proxmaster/infra/ops/breakglass/ssh-breakglass-disable.sh 22 10.13.13.0/24
```

