// Package technitium holds the resource specs.
package technitium

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ilijamt/terraform-provider-technitium/internal/framework"
)

// Specs returns the resource specs.
func Specs() []framework.Spec {
	specs := append([]framework.Spec{zone, recordSOA}, records...)
	return append(specs, settings...)
}

func Resources() []func() resource.Resource {
	var out []func() resource.Resource
	for _, s := range Specs() {
		out = append(out, framework.NewResource(s))
	}
	return out
}
