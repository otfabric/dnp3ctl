// SPDX-License-Identifier: MIT

package dnp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"

	dnp3 "github.com/otfabric/go-dnp3"
	"github.com/otfabric/go-dnp3/app"

	"github.com/otfabric/dnp3ctl/internal/errs"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// WrapError classifies an error of go-dnp3 as a typed CLI error with a stable kind and
// code. A typed error is returned as it is.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	var (
		iinErr  *dnp3.IINError
		cmdErr  *dnp3.CommandError
		fileErr *dnp3.FileError
		authErr *dnp3.AuthError
	)
	switch {
	case errors.As(err, &cmdErr):
		return errs.New(errs.KindRefused, errs.CodeCommandRefused,
			fmt.Sprintf("the outstation refused the control: %s index %d: %s", cmdErr.GV, cmdErr.Index, cmdErr.Status), err)
	case errors.As(err, &iinErr):
		return errs.New(errs.KindRefused, iinCode(iinErr.IIN),
			fmt.Sprintf("the outstation refused %s: %s", iinErr.Function, (iinErr.IIN&dnp3.IINErrorBits).String()), err)
	case errors.As(err, &fileErr):
		msg := fmt.Sprintf("the outstation refused file operation %s: %s", fileErr.Op, fileErr.Status)
		if fileErr.Text != "" {
			msg += " (" + fileErr.Text + ")"
		}
		return errs.New(errs.KindRefused, errs.CodeFileRefused, msg, err)
	case errors.As(err, &authErr):
		return errs.New(errs.KindRefused, errs.CodeAuthenticationFailed,
			fmt.Sprintf("secure authentication failed: %s (user %d)", authErr.Code, authErr.User), err)
	case errors.Is(err, dnp3.ErrAuthentication):
		return errs.New(errs.KindRefused, errs.CodeAuthenticationFailed,
			fmt.Sprintf("secure authentication failed: %v", err), err)
	}
	var (
		certErr  *tls.CertificateVerificationError
		unknown  x509.UnknownAuthorityError
		hostname x509.HostnameError
		record   tls.RecordHeaderError
	)
	switch {
	case errors.Is(err, context.Canceled):
		return errs.New(errs.KindInterrupted, errs.CodeContextCanceled, "interrupted", err)
	case errors.As(err, &certErr), errors.As(err, &unknown), errors.As(err, &hostname), errors.As(err, &record):
		return errs.New(errs.KindTransport, errs.CodeTransportTLSFailed, fmt.Sprintf("TLS error: %v", err), err)
	case errors.Is(err, dnp3.ErrConnectFailed):
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, fmt.Sprintf("connection error: %v", err), err)
	case errors.Is(err, dnp3.ErrConnectionLost), errors.Is(err, dnp3.ErrNotConnected), errors.Is(err, dnp3.ErrClosed):
		return errs.New(errs.KindTransport, errs.CodeConnectionLost, fmt.Sprintf("connection error: %v", err), err)
	case errors.Is(err, dnp3.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return errs.New(errs.KindTimeout, errs.CodeRequestTimeout, fmt.Sprintf("timeout: no answer from the outstation: %v", err), err)
	case errors.Is(err, dnp3.ErrProtocol):
		return errs.New(errs.KindProtocol, errs.CodeProtocolViolation, fmt.Sprintf("protocol error: %v", err), err)
	case errors.Is(err, dnp3.ErrBusy):
		return errs.New(errs.KindProtocol, errs.CodeRequestBusy, fmt.Sprintf("protocol error: %v", err), err)
	case errors.Is(err, dnp3.ErrInvalidOption), errors.Is(err, app.ErrInvalidValue):
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("invalid input: %v", err), err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return errs.New(errs.KindTransport, errs.CodeConnectionLost, fmt.Sprintf("connection error: %v", err), err)
	}
	return errs.Internal(err)
}

// iinCode returns the error code of the most specific IIN2 error bit.
func iinCode(iin app.IIN) string {
	switch {
	case iin.Has(app.IINNoFuncCodeSupport):
		return errs.CodeNoFuncCodeSupport
	case iin.Has(app.IINObjectUnknown):
		return errs.CodeObjectUnknown
	case iin.Has(app.IINAlreadyExecuting):
		return errs.CodeAlreadyExecuting
	default:
		return errs.CodeParameterError
	}
}

// EmbeddedErrorInfo renders err as the error object of a row or record in a result.
func EmbeddedErrorInfo(err error) *types.ErrorInfo {
	if err == nil {
		return nil
	}
	if e, ok := errs.As(WrapError(err)); ok {
		return &types.ErrorInfo{Kind: string(e.Kind), Code: e.Code, Message: e.Error()}
	}
	return &types.ErrorInfo{Kind: string(errs.KindInternal), Code: errs.CodeInternalUnexpected, Message: err.Error()}
}

// Fatal reports whether err ends the session, so that a command repeating a request (ping)
// should stop instead of trying again.
func Fatal(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, context.Canceled), errors.Is(err, dnp3.ErrConnectionLost), errors.Is(err, dnp3.ErrNotConnected), errors.Is(err, dnp3.ErrClosed):
		return true
	}
	var iinErr *dnp3.IINError
	return !errors.As(err, &iinErr) && !errors.Is(err, dnp3.ErrTimeout) && !errors.Is(err, context.DeadlineExceeded)
}
