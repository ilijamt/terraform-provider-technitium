package framework

import (
	"fmt"
	"strings"
)

func Record(recordType string, data ...Attr) Spec {
	attrs := []Attr{
		{Name: "zone", Param: "zone", Type: String, Required: true, Identity: true, Create: true, Update: true,
			Description: "The authoritative zone that holds the record."},
		{Name: "domain", Param: "domain", Type: String, Key: "name", Compare: DomainName, Required: true, Identity: true, Create: true, Update: true,
			Description: "The record's domain name, for example `www.example.com`. Use the zone name for a record at the zone apex."},
		{Name: "ttl", Param: "ttl", Type: Int64, Key: "ttl", Create: true, Update: true,
			Description: "The record TTL in seconds. The records at one name and type share one TTL, so a value different from theirs is refused, and changing it on a record that has theirs changes it on all of them. Defaults to their TTL, or else to the default record TTL in the server settings."},
		{Name: "comments", Param: "comments", Type: String, Key: "comments", Create: true, Update: true, Default: "",
			Description: "Comments stored with the record."},
		{Name: "expiry_ttl", Param: "expiryTtl", Type: Int64, Key: "expiryTtl", Create: true, Update: true, Default: 0,
			Description: "Deletes the record automatically this many seconds after it was last modified. Terraform then creates it again on the next apply. Defaults to `0`, which keeps the record."},
	}
	for _, a := range data {
		a.Create, a.Update = true, true
		attrs = append(attrs, a)
	}
	attrs = append(attrs, Attr{Name: "disable", Param: "disable", Type: Bool, Key: "disabled", Update: true, Default: false,
		Description: "Disable the record without deleting it. A DNSSEC signed zone refuses disabled records, and a zone that holds one cannot be signed. Defaults to `false`."})

	return Spec{
		Name:        "record_" + strings.ToLower(recordType),
		Description: fmt.Sprintf("Manages a single `%s` record in an authoritative zone.", recordType),
		Create:      "/api/zones/records/add",
		Read:        "/api/zones/records/get",
		Update:      "/api/zones/records/update",
		UpdateOnce:  true,
		Delete:      "/api/zones/records/delete",
		List:        "records",
		Shared:      "ttl",
		Fixed:       map[string]string{"type": recordType},
		Attrs:       attrs,
	}
}
