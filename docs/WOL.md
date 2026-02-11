# Wake-on-LAN (WOL) — Research & Integration Guide for Nudged

Version: 1.0 · Date: 2026-02-11

TL;DR
- Wake-on-LAN (WOL) sends a small "magic packet" (6×0xFF + 16×MAC) as an L2 broadcast to wake sleeping/powered-off hosts whose NIC/firmware supports it.
- WOL is a link-layer broadcast technology — it generally does not traverse routers or IP-only VPN tunnels like WireGuard/Netbird without a local relay/subnet router.
- For Nudged, prefer a relay/subnet-agent on the target LAN (or IPMI/Redfish for servers). Implement a `pkg/wol` utility for sending magic packets and a hub-side handler that calls relays when Hub cannot reach the LAN directly.

## 1. How WOL works
- Packet format: 6 bytes of 0xFF followed by 16 repetitions of the 6-byte target MAC address (total 102 bytes). Optionally a 6-byte SecureOn password can follow.
- Transport: Sent as an Ethernet broadcast. Commonly carried inside a UDP datagram to ports 7 or 9 — receivers typically look at the payload pattern rather than UDP port.
- NIC/firmware: NIC must be powered and configured to wake on magic packets. BIOS/UEFI and NIC power management must allow standby power to the PHY.

## 2. OS tooling & persistence
### Linux
- Quick checks and enable:
  - `ethtool <iface>` shows `Wake-on:` flags; `ethtool -s <iface> wol g` enables magic-packet wake.
- Persistence strategies:
  - Systemd unit that runs `ethtool -s` on boot.
  - `systemd-networkd` `.network` file: `WakeOnLan=yes`.
  - NetworkManager: `nmcli connection modify <name> 802-3-ethernet.wake-on-lan magic` or keyfile config.
- Wireless: WoWLAN support is limited; many Wi‑Fi NICs do not support wake-from-power-off (S5).

### Windows
- Device Manager: NIC → Properties → Power Management → allow wake. NIC advanced settings often include Wake-on-Magic-Packet.
- `powercfg` utilities can help debug. Driver/vendor utilities may be required for persistence.

### macOS
- Energy Saver / System Settings: "Wake for network access" (WOMP). Behavior varies by Mac model; full S5 wake may not be supported on all hardware.

## 3. Network limitations
- L2 broadcast scope: Magic packets are L2 broadcasts and do not cross routers by default.
- Directed broadcast: Routers may be configured to forward directed broadcasts (e.g., `10.0.1.255`) but many networks disable this for security.
- NAT/WAN: Sending magic packets over the public Internet to wake a host behind NAT generally fails unless the remote router supports WOL proxying or static ARP/port forwarding hacks.
- VPNs (WireGuard/Netbird): WireGuard is an IP tunnel and does not forward Ethernet broadcasts. Magic packets will not reach a host on the remote LAN unless a subnet relay (an agent on the remote LAN) receives a VPN request and emits a local L2 broadcast.

## 4. Practical strategies for Nudged
1. Direct WOL from Hub
   - Works when Hub shares the L2/VLAN with the agent host.
   - Hub uses `pkg/wol` to compute broadcast and send the magic packet.
2. Router-assisted WOL
   - Use router's WOL API or configure directed broadcast forwarding / static ARP with careful security.
   - Requires admin access to the remote gateway.
3. VPN + subnet relay (recommended for Netbird)
   - Deploy a small "relay" (subnet-agent) on the remote LAN. Hub sends an authenticated RPC over the VPN to the relay which broadcasts the magic packet locally.
   - This mirrors how Tailscale/WireGuard-based solutions implement WOL.
4. IPMI / Redfish / BMC for servers
   - Prefer authenticated OOB management APIs for servers. More reliable and auditable than WOL.
5. Always-on relay device
   - Use a low-power always-on box (Raspberry Pi, home router with custom firmware) to perform WOL/agent starting.

Recommended Nudged flow (Hub):
- Try local WOL if Hub on same L2.
- If not, call registered subnet relay over the control plane (authenticated) to emit local magic packet.
- Wait and poll for agent to reconnect; if fails, try fallback (IPMI/Redfish) or surface error.

## 5. Security considerations
- Any host with network access to the broadcast domain can issue WOL — unauthorized parties could power-on machines.
- Mitigations:
  - Only accept wake requests from authenticated clients.
  - Require Hub-to-relay mutual TLS or pre-shared tokens.
  - Rate-limit and log wake requests; audit origins.
  - Prefer IPMI/Redfish with access control for servers.
  - Use SecureOn if NIC supports it (note: password is sent in packet, not encrypted).

## 6. Go implementation options
- Libraries to consider:
  - `github.com/mdlayher/wol` — robust, supports raw Ethernet + UDP, SecureOn, interface selection.
  - `github.com/sabhiram/go-wol` — simple CLI + library, supports broadcast and interface selection.
  - `github.com/j-keck/wol` — minimal magic-packet sender, suitable for small dependencies.

### Minimal approach (drop-in)
- Build magic packet: 6×0xFF + 16×MAC.
- Send via UDP to broadcast IP (255.255.255.255 or subnet broadcast) on port 7/9.
- To choose broadcast address: inspect `net.Interfaces()` → `Interface.Addrs()` to find IPv4 and mask, then compute broadcast = ip | ^mask.
- UDP broadcast requires `SO_BROADCAST` (set via raw socket or `net.UDPConn` + syscall trick); raw Ethernet requires root/capabilities.

### Example sketch (developer-friendly)
(See `docs/WOL.md` file for a ready code snippet.)

## 7. Retry/backoff & timeouts
- After sending WOL, wait an initial delay (5–15s), poll for agent control-plane reconnect.
- Exponential backoff for retries (15s, 30s, 60s, 120s) up to a configurable max (e.g., 10 minutes).
- Log attempts and return a clear structured error if all retries fail.

## 8. Suggested repo changes
- `pkg/wol/wol.go` — implement `Send(mac string, iface string, bcast string, port int) error` and helpers to compute broadcast and validate NIC support.
- `pkg/server/wake_handler.go` — expose `POST /agents/{id}/wake` or `POST /wake` API that: validates auth, looks up agent MAC/relay, calls `pkg/wol` or forwards to registered relay, and orchestrates retries.
- `scripts/wol-send` — small operator CLI.
- `docs/WOL.md` — this document.

## 9. Testing & operational checklist
- Verify NIC/BIOS: ensure `ethtool` shows `Wake-on: g` and enable if needed.
- Test local on-LAN WOL: send magic packet and observe NIC LED and boot.
- Test relay: deploy relay into remote LAN and validate Hub→relay RPC and relay broadcast.
- Test failure modes: VLAN changes, DHCP lease changes (router ARP), Wi‑Fi limitations.

## 10. References
- RFC-style and vendor docs (search terms): "magic packet" "Wake-on-LAN" "ethernet broadcast" "SecureOn" "directed broadcast" "ethtool wol".
- Go packages: `github.com/mdlayher/wol`, `github.com/sabhiram/go-wol`, `github.com/j-keck/wol`.

---

*Document generated by Copilot assistant — ready to be refined with hardware-specific notes.*
