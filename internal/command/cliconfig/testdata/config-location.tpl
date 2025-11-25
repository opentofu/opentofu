{{$index := .Index}}
host "config{{$index}}.example.com" {
  services = {
    "modules.v{{$index}}" = "https://config{{$index}}.example.com/",
  }
}
{{range .Subdomains}}
host "{{.}}.example.com" {
  services = {
    "modules.v{{$index}}" = "https://{{.}}.example.com/",
  }
}
{{end}}