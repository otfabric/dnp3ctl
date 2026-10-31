// SPDX-License-Identifier: MIT

package errs

// Kind is a coarse failure category for exit-code mapping and JSON error.kind.
type Kind string

const (
	KindUsage        Kind = "usage"
	KindInvalidInput Kind = "invalid_input"
	KindTransport    Kind = "transport"
	KindTimeout      Kind = "timeout"
	KindProtocol     Kind = "protocol"
	// KindRefused is a request the outstation answered and refused: IIN2 error bits in
	// the response, a control echoed with a status other than SUCCESS, a refused file
	// operation or a failed secure authentication.
	KindRefused     Kind = "refused"
	KindOutput      Kind = "output"
	KindInternal    Kind = "internal"
	KindInterrupted Kind = "interrupted"
)
