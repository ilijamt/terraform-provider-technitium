package framework

import (
	"fmt"
	"strings"
)

// Record returns the spec of the record_<type> resource for a DNS record
// type. Every record shares zone, domain, ttl, comments, expiry_ttl and
// disable; data are the type's own attributes, which both the add and the
// update call accept.
func Record(recordType string, data ...Attr) Spec {
	attrs := []Attr{
		{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Create: true, Update: true,
			Description: "The authoritative zone that holds the record."},
		{Name: "domain", Param: "domain", Type: String, Key: "name", Required: true, Identity: true, Create: true, Update: true,
			Description: "The record's domain name, for example `www.example.com`. Use the zone name for a record at the zone apex."},
		{Name: "ttl", Param: "ttl", Type: Int64, Key: "ttl", Create: true, Update: true,
			Description: "The record TTL in seconds. Defaults to the default record TTL in the server settings."},
		{Name: "comments", Param: "comments", Type: String, Key: "comments", Create: true, Update: true,
			Description: "Comments stored with the record."},
		{Name: "expiry_ttl", Param: "expiryTtl", Type: Int64, Key: "expiryTtl", Create: true, Update: true,
			Description: "Deletes the record automatically this many seconds after it was last modified. Terraform then creates it again on the next apply."},
	}
	for _, a := range data {
		a.Create, a.Update = true, true
		attrs = append(attrs, a)
	}
	attrs = append(attrs, Attr{Name: "disable", Param: "disable", Type: Bool, Key: "disabled", Update: true,
		Description: "Disable the record without deleting it. Defaults to `false`."})

	return Spec{
		Name:        "record_" + strings.ToLower(recordType),
		Description: fmt.Sprintf("Manages a single `%s` record in an authoritative zone.", recordType),
		Create:      "/api/zones/records/add",
		Read:        "/api/zones/records/get",
		Update:      "/api/zones/records/update",
		Delete:      "/api/zones/records/delete",
		List:        "records",
		Fixed:       map[string]string{"type": recordType},
		Attrs:       attrs,
	}
}
