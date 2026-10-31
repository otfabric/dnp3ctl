// SPDX-License-Identifier: MIT

package errs

// Stable machine-oriented codes for JSON error.code and scripts.
const (
	CodeInvalidFormat          = "invalid_format"
	CodeInvalidInput           = "invalid_input"
	CodeInvalidURL             = "invalid_url"
	CodeInvalidFlagCombination = "invalid_flag_combination"
	CodeInvalidConfig          = "invalid_config"
	CodeInvalidKey             = "invalid_key"

	CodeTransportConnectFailed = "transport_connect_failed"
	CodeTransportTLSFailed     = "transport_tls_failed"
	CodeConnectionLost         = "connection_lost"
	CodeListenFailed           = "listen_failed"

	// The outstation did not answer a request, a confirmation or a link frame in time.
	CodeRequestTimeout = "request_timeout"

	CodeProtocolViolation = "protocol_violation"
	CodeRequestBusy       = "request_busy"

	// The outstation refused the request with IIN2 bits in its response.
	CodeNoFuncCodeSupport = "no_func_code_support"
	CodeObjectUnknown     = "object_unknown"
	CodeParameterError    = "parameter_error"
	CodeAlreadyExecuting  = "already_executing"
	// The outstation echoed a control with a status other than SUCCESS.
	CodeCommandRefused = "command_refused"
	// The outstation refused a file operation.
	CodeFileRefused = "file_refused"
	// Secure authentication failed: wrong key or user, or not configured on one side.
	CodeAuthenticationFailed = "authentication_failed"

	// Context was canceled (not invalid user input).
	CodeContextCanceled = "context_canceled"

	CodeJSONEncodeFailed       = "json_encode_failed"
	CodeTableRenderFailed      = "table_render_failed"
	CodeOutputWriteFailed      = "output_write_failed"
	CodeOutputFileWriteFailed  = "output_file_write_failed"
	CodeOutputFileCreateFailed = "output_file_create_failed"
	CodeInputFileOpenFailed    = "input_file_open_failed"

	CodeInternalUnexpected = "internal_unexpected"
)
