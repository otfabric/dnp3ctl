// SPDX-License-Identifier: MIT

package dnp

import (
	"github.com/otfabric/go-dnp3/app"

	"github.com/otfabric/dnp3ctl/internal/config"
	"github.com/otfabric/dnp3ctl/internal/types"
)

// KnownObjects returns every group and variation go-dnp3 encodes and decodes, in order.
func KnownObjects() []app.GroupVariation {
	var out []app.GroupVariation
	for g := 0; g < 256; g++ {
		for v := 0; v < 256; v++ {
			if gv := app.GV(uint8(g), uint8(v)); gv.Known() {
				out = append(out, gv)
			}
		}
	}
	return out
}

// Objects lists the objects of cfg.
func Objects(cfg *config.ObjectsConfig) *types.ObjectsResult {
	res := &types.ObjectsResult{Objects: []types.ObjectInfo{}}
	for _, gv := range KnownObjects() {
		if cfg.Group != 0 && gv.Group() != cfg.Group || cfg.Events && !gv.IsEvent() {
			continue
		}
		size, _ := gv.Size()
		res.Objects = append(res.Objects, types.ObjectInfo{
			Object: gv.String(), Group: gv.Group(), Variation: gv.Variation(), Name: gv.Name(), Event: gv.IsEvent(), Size: size,
		})
	}
	res.Count = len(res.Objects)
	return res
}
