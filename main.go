// Command claudeproxy routes Claude Code traffic: Anthropic models are
// forwarded untouched, other models go to providers chosen by configuration.
package main

import "github.com/JLugagne/claudeproxy/cmd/claudeproxy"

func main() {
	claudeproxy.Execute()
}
