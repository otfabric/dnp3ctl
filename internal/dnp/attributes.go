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

// ParseAttributeVariation parses --variation of a read: all, list, a number 1-255 or, in
// set 0, a name.
func ParseAttributeVariation(s string) (uint8, error) {
	return parseVariation(s, 0, false)
}

// parseVariation parses --variation in set; write leaves out all and list, which cannot be
// written.
func parseVariation(s string, set uint16, write bool) (uint8, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	hint := "all, list, a number 1-255 or, in set 0, a name such as manufacturer"
	if write {
		hint = "a number 1-253 or, in set 0, a name such as location"
	}
	if in == "" && !write {
		return app.AttrAllAttributes, nil
	}
	v, found := uint8(0), false
	if n, err := strconv.ParseUint(in, 10, 8); err == nil && n != 0 {
		v, found = uint8(n), true
	}
	for code, name := range AttributeNames {
		if found || name != in {
			continue
		}
		if set != 0 && code != app.AttrAllAttributes && code != app.AttrListOfVariations {
			return 0, fmt.Errorf("invalid --variation %q: names are those of set 0; in set %d give the variation number", s, set)
		}
		v, found = code, true
	}
	if !found || write && v >= app.AttrAllAttributes {
		return 0, fmt.Errorf("invalid --variation %q: use %s (see docs/json-output.md)", s, hint)
	}
	return v, nil
}

// attributeName names a variation of a set: the names of the standard apply to set 0.
func attributeName(set uint16, v uint8) string {
	if set == 0 || v == app.AttrListOfVariations || v == app.AttrAllAttributes {
		return AttributeName(v)
	}
	return "attribute-" + strconv.Itoa(int(v))
}

// attributeValue decodes the value of an attribute for the results.
func attributeValue(a app.Attribute) any { return AttributeValue(a, 0) }

// AttributeValue decodes the value of an attribute of set for the results.
func AttributeValue(a app.Attribute, set uint16) any {
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
				out = append(out, types.AttributeListEntry{Variation: e.Variation, Name: attributeName(set, e.Variation), Writable: e.Writable})
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
	_, err := parseVariation(cfg.Variation, cfg.Set, false)
	return err
}

// Attributes reads device attributes (group 0).
func Attributes(ctx context.Context, cfg *config.AttributesConfig, trace io.Writer) (*types.AttributesResult, error) {
	variation, err := parseVariation(cfg.Variation, cfg.Set, false)
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
		if cfg.Set == 0 {
			attrs, rerr = s.ReadAttributes(ctx, variation)
		} else {
			attrs, rerr = s.ReadAttributeSet(ctx, cfg.Set, variation)
		}
		return rerr
	}); err != nil {
		return nil, WrapError(err)
	}
	res := &types.AttributesResult{Target: s.Target.String(), Outstation: s.Outstation, Set: cfg.Set, Attributes: []types.Attribute{}}
	for _, v := range sortedKeys(attrs) {
		res.Attributes = append(res.Attributes, describeAttribute(cfg.Set, v, attrs[v]))
	}
	res.Count = len(res.Attributes)
	return res, nil
}

// describeAttribute is the result form of the attribute v of set.
func describeAttribute(set uint16, v uint8, a app.Attribute) types.Attribute {
	return types.Attribute{Variation: v, Name: attributeName(set, v), DataType: a.Type.String(), Value: AttributeValue(a, set)}
}

// AttributeDataTypes describes the values of --data-type of `client write-attribute`.
func AttributeDataTypes() map[string]string {
	return map[string]string{
		"vstr":  "visible string",
		"uint":  "unsigned integer, 0-4294967295",
		"int":   "signed integer, 32 bits",
		"float": "floating point",
	}
}

// ParseAttribute converts --value of the type --data-type into an attribute.
func ParseAttribute(value, dataType string) (app.Attribute, error) {
	switch t := strings.ToLower(strings.TrimSpace(dataType)); t {
	case "", "vstr", "string":
		if value == "" || len(value) > 255 {
			return app.Attribute{}, fmt.Errorf("invalid --value: a visible string holds 1-255 octets")
		}
		return app.StringAttribute(value), nil
	case "uint":
		n, err := strconv.ParseUint(strings.TrimSpace(value), 0, 32)
		if err != nil {
			return app.Attribute{}, fmt.Errorf("invalid --value %q for --data-type uint: use 0-4294967295", value)
		}
		return app.UintAttribute(uint32(n)), nil
	case "int":
		n, err := strconv.ParseInt(strings.TrimSpace(value), 0, 32)
		if err != nil {
			return app.Attribute{}, fmt.Errorf("invalid --value %q for --data-type int: use a 32-bit integer", value)
		}
		return app.IntAttribute(int32(n)), nil
	case "float":
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return app.Attribute{}, fmt.Errorf("invalid --value %q for --data-type float: use a number", value)
		}
		return app.FloatAttribute(f), nil
	default:
		return app.Attribute{}, fmt.Errorf("invalid --data-type %q: use vstr, uint, int or float", dataType)
	}
}

func writeAttributePlan(cfg *config.WriteAttributeConfig) (uint8, app.Attribute, error) {
	v, err := parseVariation(cfg.Variation, cfg.Set, true)
	if err != nil {
		return 0, app.Attribute{}, err
	}
	a, err := ParseAttribute(cfg.Value, cfg.DataType)
	return v, a, err
}

// CheckWriteAttribute validates the configuration of `client write-attribute`.
func CheckWriteAttribute(cfg *config.WriteAttributeConfig) error {
	if err := Validate(&cfg.ConnConfig); err != nil {
		return err
	}
	_, _, err := writeAttributePlan(cfg)
	return err
}

// WriteAttribute writes a device attribute of a set.
func WriteAttribute(ctx context.Context, cfg *config.WriteAttributeConfig, trace io.Writer) (*types.ConfirmedResult, error) {
	v, a, err := writeAttributePlan(cfg)
	if err != nil {
		return nil, err
	}
	set, attr := int(cfg.Set), describeAttribute(cfg.Set, v, a)
	res := &types.ConfirmedResult{Request: "write-attribute", Set: &set, Attribute: &attr}
	return confirmed(ctx, &cfg.ConnConfig, trace, res, func(ctx context.Context, s *Session) error {
		if cfg.Set == 0 {
			return s.WriteAttribute(ctx, v, a)
		}
		return s.WriteAttributeSet(ctx, cfg.Set, v, a)
	})
}
