# Process exit codes

`dnp3ctl` uses stable exit codes for scripting. A fatal error is rendered once: as one `error:` line on stderr, or as a single JSON `error` object on stdout when `--format json` is active.

| Code | Meaning |
|------|---------|
| 0 | Success (`ExitOK`) |
| 2 | Usage (`KindUsage`): unknown command or flag, a missing required flag, a value a flag cannot hold |
| 3 | Invalid input (`KindInvalidInput`): a flag value, combination of flags, environment variable, key file or outstation model that does not make sense. Nothing was sent |
| 4 | Transport (`KindTransport`): the connection cannot be made (refused, unreachable, TLS failure, serial port that does not open), is lost, or the server cannot listen |
| 5 | Timeout (`KindTimeout`): the outstation did not answer a request, a link frame or a confirmation in time |
| 6 | Refused or protocol error (`KindRefused`, `KindProtocol`): the outstation refused the request (IIN2 error bits, a control status other than `SUCCESS`, a refused file operation, a failed secure authentication), or violated the protocol |
| 7 | Partial success (`ExitPartial`): a **valid** result was written to stdout, and it holds failures: a poll or read the outstation answered with values while it also reported an error in its IIN, or a failed measurement of `client ping` |
| 10 | Output / internal (`KindOutput`, `KindInternal`): the result could not be written, the command was interrupted before it had a result, or something unexpected happened |

**Kind vs code:** `kind` drives the exit code; `code` in the JSON error object is stable for machines. Do not parse the human `message`.

| `kind` | `code` values |
|--------|---------------|
| `usage`, `invalid_input` | `invalid_input`, `invalid_format`, `invalid_url`, `invalid_flag_combination`, `invalid_config`, `invalid_key`, `input_file_open_failed`, `output_file_create_failed` |
| `transport` | `transport_connect_failed`, `transport_tls_failed`, `connection_lost`, `listen_failed` |
| `timeout` | `request_timeout` |
| `refused` | `no_func_code_support` (IIN2.0), `object_unknown` (IIN2.1), `parameter_error` (IIN2.2), `already_executing` (IIN2.4), `command_refused` (a control echoed with another status than `SUCCESS`; the message names it), `file_refused`, `authentication_failed` |
| `protocol` | `protocol_violation`, `request_busy` |
| `output` | `json_encode_failed`, `table_render_failed`, `output_write_failed`, `output_file_write_failed`, `output_file_create_failed` |
| `internal` | `context_canceled`, `internal_unexpected` |

**Partial exit:** only after a success payload was written. A failure to connect, to validate or to write is fatal and uses the codes above, never `7`. A poll or read that the outstation refuses without returning any value is fatal (`6`), not partial.

**Streaming commands:** `client monitor`, `client ping` and `server start` write one record at a time. Stopping them with Ctrl-C (SIGINT) or SIGTERM ends the stream normally and does not change the exit code. `client ping` keeps measuring after a failed measurement, which it reports as a record carrying an `error`, and exits `7` when at least one failed; a measurement that fails because the connection is gone ends it with the fatal code of that error. `client monitor` exits `0` when it ends by `--count`, `--duration` or a signal, and `4` when the connection is lost and `--no-reconnect` is set.

**Commands that change an outstation:** `client operate`, `client time-sync`, `client restart`, `client unsolicited`, `client assign-class`, `client freeze`, `client file put` and `client file rm` validate everything before sending; a rejected invocation exits `3` and has sent nothing. A refusal by the outstation exits `6`, a missing answer `5`. Controls, freezes, restarts and file writes are never sent a second time, also not with `--retries`; after a timeout (`5`) a control may or may not have been executed.

**`--debug`:** adds the frame trace and, after a fatal error, the chain of causes (`cause: …` lines) on stderr. It does not change stdout or the exit code.
