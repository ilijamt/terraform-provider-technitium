# zone/domain/character_strings_base64/text/split_text, empty values left unset
terraform import technitium_record_txt.hello txt.example.com/hello.txt.example.com//hello/

# or a JSON object of identity attributes, for values containing "/"
terraform import technitium_record_txt.spf '{"zone":"txt.example.com","domain":"txt.example.com","text":"v=spf1 ip4:192.168.178.0/24 -all"}'
