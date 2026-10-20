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

`dnp3ctl` implements Secure Authentication version 5 (IEEE 1815-2012 clause 7) through go-dnp3: the outstation challenges critical requests and the master answers with an HMAC over them. The session keys come from one of two places:

- **An update key** (recommended): the master negotiates fresh session keys with the session key change procedure, wrapped with the update key (AES-128 or AES-256 key wrap), on every connection and every `--sa-key-change-interval` seconds (default 15 minutes). The update key has 16 or 32 octets
- **A pre-shared session key**: used directly for every challenge, for outstations without key change. It has at least 16 octets

Keys are handled as follows:

- They are read from files (`--sa-update-key-file`, `--sa-key-file`) or environment variables (`DNP3CTL_SA_UPDATE_KEY`, `DNP3CTL_SA_KEY`), as hex, and **never** accepted as a flag: a flag would end up in the shell history and the process list. Keep key files readable by you alone (`chmod 600`), and prefer the files to the environment on shared hosts. Give one kind of key, not both
- The MAC defaults to HMAC-SHA-256 truncated to 16 octets; the HMAC-SHA-1 variants exist for older outstations
- **Aggressive mode** (`--sa-aggressive`, `DNP3CTL_SA_AGGRESSIVE`; clause 7.5.2.2.4): once the outstation has challenged one request, the master sends further critical requests with their MAC over that last challenge, without waiting for a new one. It saves a round trip per control and needs a key like any secure authentication; the outstation checks the MAC and the challenge sequence number. Within one command it applies from the second critical request on: in `client operate --mode sbo` the OPERATE after the challenged SELECT
- **Security statistics** (clause 7.6): an outstation with secure authentication counts, per master, its authentication failures, session key changes, critical messages and the other statistics of the standard. `client read --object g121v1` shows them by name; the simulated outstation keeps them and reports g122 events in class 1 every 10 increments. A rising `authentication failures` or `unexpected messages` count is worth a look
- Interoperability: go-dnp3's interop suite verifies the challenge, the session key change and the security statistics against DSC go-dnp3 in both directions; opendnp3 3.x and Step Function I/O's dnp3 have no SAv5. Aggressive mode is verified between go-dnp3's own master and outstation only, and dnp3ctl against its simulated outstation, as no reference stack implements it
- The update key change and user management are not implemented. SAv5 authenticates requests; it does not encrypt them: combine it with TLS where confidentiality matters
- The simulated outstation verifies with the same key (`server start --sa-update-key-file` or `--sa-key-file`); it drops the session keys when a connection ends, and challenges the default critical function codes of go-dnp3 (controls, writes, restarts, freezes, assign class, opening and deleting files, among others)

### The outstation connects (dual end point)

With `client … --listen` dnp3ctl waits for an outstation to connect to it (IEEE 1815-2012 annex A.3), and with `server start --dial` the simulated outstation connects to its master. A listening master takes the **first connection** that arrives on its port as the outstation, and a dialing outstation serves whoever answers at the address:

- Listen on the interface the outstation comes from (`--listen 10.0.0.1`), not on every interface, and only for as long as the command runs (it closes the port when it ends)
- Use TLS where the network is not trusted: `--listen` with `--tls-cert`, `--tls-key` and `--tls-ca`, so that the outstation must present a certificate of that CA; `server start --dial tls://…` verifies the master's certificate against `--tls-ca` (the system roots by default) and presents `--tls-cert` when given
- Link addresses still apply: a connection from a station with another link address gets no answer, and Secure Authentication works as on a connection the master makes

### File Transfer Credentials

`--file-user` sends credentials with AUTHENTICATE_FILE. The password comes from `--file-password-file` or `DNP3CTL_FILE_PASSWORD`, never from a flag. DNP3 sends it in clear: use it over TLS or on a trusted network only.

### Commands That Change an Outstation

- **`client operate`** operates outputs and changes set-points. On real equipment this opens and closes breakers, starts pumps and changes what a plant does. Know what an index controls before sending to it. Everything is validated before anything is sent, and a control is sent exactly once, also with `--retries`
- **`client time-sync`** changes the outstation's clock, and with it the time stamps of its events
- **`client restart`** restarts the outstation or its DNP3 application; `--type` has no default
- **`client freeze`** overwrites frozen values, and with `--clear` resets counters. Energy counters that are reset are gone. With `--at` and `--interval` the outstation keeps freezing on its own, at every interval
- **`--broadcast`** (`client operate`, `client freeze`) reaches **every** outstation on the channel at once, also ones you did not mean, and nobody answers: there is no status to tell whether, or where, it was executed. Use it only on a channel whose outstations you know
- **`client write-attribute`** changes the outstation's device attributes, which other systems may use to identify it
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
