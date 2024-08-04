# hdhr-relay
Relay HDHomeRun connections, allowing access across networks (VLANs or VPNs).

The relay blocks any urls not related to video playback, allowing view access to be granted without the ability to
retune channels.

## Run

```bash
go install github.com/csnewman/hdhr-relay@latest
sudo env "PATH=$PATH" hdhr-relay 192.168.1.20 192.168.2.30
```
Replace `192.168.1.10` with the address of the HDHomeRun and replacee `192.168.2.30` with the address of the machine
hosting the relay.

The command requires elevated permissions, as it binds to ports `80`, `65001` and `5004`.
