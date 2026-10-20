# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `dnp3ctl`, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@otfabric.io**, or open a private advisory on GitHub (the [Security](https://github.com/otfabric/dnp3ctl/security) tab → **Advisories** → **Report a vulnerability**), with:

- A description of the vulnerability
- Steps to reproduce the issue
- The affected version(s) (`dnp3ctl version` output)
- Any potential impact you've identified

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation within 7 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | Best effort |

## Security Considerations

### DNP3 Has No Built-In Security

Plain DNP3 carries no encryption, and without Secure Authentication no authentication: anyone who can reach an outstation can poll and operate it, and anyone on the path can read or alter the traffic. `dnp3ctl` does not change that.

- **Use it only on outstations and networks you own or are authorized to test**
- **Keep SCADA traffic on isolated, trusted networks**; never expose port 20000 to the Internet
- Use TLS and Secure Authentication where the outstation supports them (below), or a VPN or secured gateway in front of it

### TLS

`dnp3ctl` supports DNP3 over TLS (IEEE 1815-2012 Annex B, IEC 62351-3): `tls://` URLs with `--tls-ca`, and `--tls-cert` / `--tls-key` for mutual authentication; the server takes `--tls-cert`, `--tls-key` and `--tls-client-ca`. TLS 1.2 is the minimum.

- `--tls-insecure` turns off verification of the outstation's certificate. It is for test setups; do not use it against an outstation that matters
- Private keys are read from the files you name and are never written or printed. Keep them readable by you alone

### Secure Authentication v5

`dnp3ctl` implements Secure Authentication version 5 (IEEE 1815-2012 clause 7) with a **pre-shared session key**, through go-dnp3: the outstation challenges critical requests and the master answers with an HMAC over them.

- The key is read from `--sa-key-file` or `DNP3CTL_SA_KEY`, as hex, and **never** accepted as a flag: a flag would end up in the shell history and the process list. Keep key files readable by you alone (`chmod 600`), and prefer the file to the environment on shared hosts
- The key must have at least 16 octets. The MAC defaults to HMAC-SHA-256 truncated to 16 octets; the HMAC-SHA-1 variants exist for older outstations
- Only the pre-shared mode is implemented: the session key change, update key change and user management procedures, and aggressive mode, are not. SAv5 authenticates requests; it does not encrypt them: combine it with TLS where confidentiality matters
- The simulated outstation verifies with the same key (`server start --sa-key-file`); it challenges the default critical function codes of go-dnp3 (controls, writes, restarts, freezes, assign class, opening and deleting files, among others)

### File Transfer Credentials

`--file-user` sends credentials with AUTHENTICATE_FILE. The password comes from `--file-password-file` or `DNP3CTL_FILE_PASSWORD`, never from a flag. DNP3 sends it in clear: use it over TLS or on a trusted network only.

### Commands That Change an Outstation

- **`client operate`** operates outputs and changes set-points. On real equipment this opens and closes breakers, starts pumps and changes what a plant does. Know what an index controls before sending to it. Everything is validated before anything is sent, and a control is sent exactly once, also with `--retries`
- **`client time-sync`** changes the outstation's clock, and with it the time stamps of its events
- **`client restart`** restarts the outstation or its DNP3 application; `--type` has no default
- **`client freeze`** overwrites frozen values, and with `--clear` resets counters. Energy counters that are reset are gone
- **`client assign-class`** changes which events the outstation reports, to every master
- **`client unsolicited`** changes what the outstation reports by itself to this master
- **`client file put`** and **`client file rm`** change the outstation's files, which may be a configuration or firmware

### Commands That Are Read-Only but Not Invisible

- **Polls and reads of events take events away.** An outstation discards events once a master has read and confirmed them. `client poll` (integrity and event), reads of event objects and `client monitor` with the link address of the regular master take those events away from it. Use a link address of your own when the outstation keeps a buffer per master
- **`client monitor`** enables unsolicited responses for its association
- **A connection with the link address of a connected master replaces that master's connection**: most outstations, and the simulated one, take it for the master reconnecting. Use a link address of your own (`--master`) next to a master that is in service
- **`discover`** connects to every address of a subnet. It sends one link layer REQUEST_LINK_STATUS per link address and no application layer request, but the connection may be logged, or counted against an outstation's connection limit. Scan production networks only with the agreement of whoever runs them

### Running a Server

- `server start` listens on **all network interfaces** and accepts **any client** by default. Use `--listen` to bind to one address, `--allow` to restrict the clients, `masters` in the model to restrict the link addresses answered, or a host firewall
- The simulated outstation executes every control it is sent on its simulated points, and takes and deletes files when the model accepts uploads. With `uploads.dir` written files are stored below that directory under their (cleaned) name; a name cannot escape the directory
- A model may reference files on disk (`files[].path`): the server serves whatever those paths name to any client it accepts

### Output

- Command output, event logs and fetched files contain process values, addresses and network details, which may be sensitive operational data. Be careful when storing or sharing them
- `--debug` prints every frame sent and received to stderr, including the authentication objects of Secure Authentication (challenges and MACs, not the key)

### Privileges

`dnp3ctl` needs no elevated privileges. `server start` on port 20000 works as a normal user; a serial port needs membership of the group that owns it (for example `dialout` on Linux), not root.

### Verifying Downloads

Release archives are published with SHA-256 checksums. Verify them as shown in the [installation instructions](README.md#installation) before installing.
