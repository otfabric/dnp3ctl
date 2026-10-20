package dnp

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/otfabric/go-dnp3/app"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// AttributeNames names the device attributes of set 0 (IEEE 1815-2012 Table 11-11), in the
// short form --variation accepts.
var AttributeNames = map[uint8]string{
	app.AttrConfigID:                    "config-id",
	app.AttrConfigVersion:               "config-version",
	app.AttrConfigBuildDate:             "config-build-date",
	app.AttrConfigLastChangeDate:        "config-last-change-date",
	app.AttrConfigDigest:                "config-digest",
	app.AttrConfigDigestAlgorithm:       "config-digest-algorithm",
	app.AttrMasterResourceID:            "master-resource-id",
	app.AttrDeviceAltitude:              "device-altitude",
	app.AttrDeviceLongitude:             "device-longitude",
	app.AttrDeviceLatitude:              "device-latitude",
	app.AttrSecondaryOperatorName:       "secondary-operator",
	app.AttrPrimaryOperatorName:         "primary-operator",
	app.AttrSystemName:                  "system-name",
	app.AttrSecureAuthVersion:           "secure-auth-version",
	app.AttrNumSecurityStats:            "security-statistics",
	app.AttrUserSpecificAttributes:      "user-specific-attributes",
	app.AttrNumMasterDataSetProtos:      "master-data-set-prototypes",
	app.AttrNumOutstationDataSetProtos:  "outstation-data-set-prototypes",
	app.AttrNumMasterDataSets:           "master-data-sets",
	app.AttrNumOutstationDataSets:       "outstation-data-sets",
	app.AttrMaxBinaryOutputsPerRequest:  "max-binary-outputs-per-request",
	app.AttrLocalTimingAccuracy:         "local-timing-accuracy",
	app.AttrTimeAccuracyDuration:        "time-accuracy-duration",
	app.AttrSupportsAnalogOutputEvents:  "analog-output-events",
	app.AttrMaxAnalogOutputIndex:        "max-analog-output-index",
	app.AttrNumAnalogOutputs:            "analog-outputs",
	app.AttrSupportsBinaryOutputEvents:  "binary-output-events",
	app.AttrMaxBinaryOutputIndex:        "max-binary-output-index",
	app.AttrNumBinaryOutputs:            "binary-outputs",
	app.AttrSupportsFrozenCounterEvents: "frozen-counter-events",
	app.AttrSupportsFrozenCounters:      "frozen-counters",
	app.AttrSupportsCounterEvents:       "counter-events",
	app.AttrMaxCounterIndex:             "max-counter-index",
	app.AttrNumCounters:                 "counters",
	app.AttrSupportsFrozenAnalogInputs:  "frozen-analog-inputs",
	app.AttrSupportsAnalogInputEvents:   "analog-input-events",
	app.AttrMaxAnalogInputIndex:         "max-analog-input-index",
	app.AttrNumAnalogInputs:             "analog-inputs",
	app.AttrSupportsDoubleBitEvents:     "double-bit-events",
	app.AttrMaxDoubleBitIndex:           "max-double-bit-index",
	app.AttrNumDoubleBitInputs:          "double-bit-inputs",
	app.AttrSupportsBinaryInputEvents:   "binary-input-events",
	app.AttrMaxBinaryInputIndex:         "max-binary-input-index",
	app.AttrNumBinaryInputs:             "binary-inputs",
	app.AttrMaxTxFragmentSize:           "max-tx-fragment-size",
	app.AttrMaxRxFragmentSize:           "max-rx-fragment-size",
	app.AttrSoftwareVersion:             "software-version",
	app.AttrHardwareVersion:             "hardware-version",
	app.AttrOwnerName:                   "owner",
	app.AttrUserLocation:                "location",
	app.AttrUserID:                      "user-id",
	app.AttrUserDeviceName:              "device-name",
	app.AttrSerialNumber:                "serial-number",
	app.AttrSubsetAndConformance:        "subset-and-conformance",
	app.AttrProductNameAndModel:         "product",
	app.AttrManufacturerName:            "manufacturer",
	app.AttrAllAttributes:               "all",
	app.AttrListOfVariations:            "list",
}

// AttributeName returns the name of a device attribute variation, or "attribute-<n>" for one
// without a name in the standard (the user-specific ones, for example).
func AttributeName(v uint8) string {
	if n, ok := AttributeNames[v]; ok {
		return n
	}
	return "attribute-" + strconv.Itoa(int(v))
}

// ParseAttributeVariation parses --variation: all, list, a number 1-255 or a name.
func ParseAttributeVariation(s string) (uint8, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	if in == "" {
		return app.AttrAllAttributes, nil
	}
	if n, err := strconv.ParseUint(in, 10, 8); err == nil && n != 0 {
		return uint8(n), nil
	}
	for v, name := range AttributeNames {
		if name == in {
			return v, nil
		}
	}
	return 0, fmt.Errorf("invalid --variation %q: use all, list, a number 1-255 or a name such as manufacturer (see docs/json-output.md)", s)
}

// attributeValue decodes the value of an attribute for the results.
func attributeValue(a app.Attribute) any {
	switch a.Type {
	case app.AttrVisibleString:
		return a.Text()
	case app.AttrUnsignedInt:
		if v, err := a.Uint(); err == nil {
			return float64(v)
		}
	case app.AttrSignedInt:
		if v, err := a.Int(); err == nil {
			return float64(v)
		}
	case app.AttrFloat:
		if v, err := a.Float(); err == nil {
			return v
		}
	case app.AttrTime:
		if len(a.Data) == 6 {
			var t app.Timestamp
			for i := 5; i >= 0; i-- {
				t = t<<8 | app.Timestamp(a.Data[i])
			}
			return timeText(t)
		}
	case app.AttrList, app.AttrExtList:
		if entries, err := a.List(); err == nil {
			out := make([]types.AttributeListEntry, 0, len(entries))
			for _, e := range entries {
				out = append(out, types.AttributeListEntry{Variation: e.Variation, Name: AttributeName(e.Variation), Writable: e.Writable})
			}
			return out
		}
	case app.AttrOctetString, app.AttrBitString:
	}
	return "0x" + hex.EncodeToString(a.Data)
}

// CheckAttributes validates the configuration of `client attributes`.
func CheckAttributes(cfg *config.AttributesConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, err := ParseAttributeVariation(cfg.Variation)
	return err
}

// Attributes reads device attributes (group 0).
func Attributes(ctx context.Context, cfg *config.AttributesConfig, trace io.Writer) (*types.AttributesResult, error) {
	variation, err := ParseAttributeVariation(cfg.Variation)
	if err != nil {
		return nil, err
	}
	s, err := Dial(ctx, &cfg.ConnConfig, trace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	var attrs map[uint8]app.Attribute
	if _, err := s.request(ctx, func(ctx context.Context) error {
		var rerr error
		attrs, rerr = s.ReadAttributes(ctx, variation)
		return rerr
	}); err != nil {
		return nil, WrapError(err)
	}
	res := &types.AttributesResult{Target: s.Target.String(), Outstation: s.Outstation, Attributes: []types.Attribute{}}
	for _, v := range sortedKeys(attrs) {
		a := attrs[v]
		res.Attributes = append(res.Attributes, types.Attribute{Variation: v, Name: AttributeName(v), DataType: a.Type.String(), Value: attributeValue(a)})
	}
	res.Count = len(res.Attributes)
	return res, nil
}
