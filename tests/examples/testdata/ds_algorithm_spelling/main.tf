resource "technitium_zone" "test" {
  zone = "ds.test"
}

resource "technitium_record_ds" "nsec3" {
  zone        = technitium_zone.test.zone
  domain      = "child.ds.test"
  key_tag     = 1
  algorithm   = "RSASHA1_NSEC3_SHA1"
  digest_type = "SHA1"
  digest      = "2BB183AF5F22588179A53B0A98631FAD1A292118"
}

resource "technitium_record_ds" "gost" {
  zone        = technitium_zone.test.zone
  domain      = "child.ds.test"
  key_tag     = 2
  algorithm   = "ECC_GOST"
  digest_type = "GOST_R_34_11_94"
  digest      = "2BB183AF5F22588179A53B0A98631FAD1A292118D3F04E8B8F6A1FB4A10A8D0D"
}
