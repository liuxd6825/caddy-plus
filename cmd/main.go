package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	// plug in Caddy modules here
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	_ "github.com/liuxd6825/caddy-plus/dynamic_sd"
)

func main() {
	caddycmd.Main()
}
