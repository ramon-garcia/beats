package actions

import (
	"fmt"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors"
	"github.com/robertkrimen/otto"
)

type javascript struct {
	vm     *otto.Otto
	script string
}

func init() {
	processors.RegisterPlugin("javascript",
		configChecked(newJavascript,
			requireFields("script"),
			allowedFields("field", "when")))
}

func newJavascript(c common.Config) (processors.Processor, error) {
	type config struct {
		script string `config:"script"`
	}

	var myconfig config
	err := c.Unpack(&myconfig)
	if err != nil {
		logp.Warn("Error unpacking config for grok")
		return nil, fmt.Errorf("fail to unpack the grok configuration: %s", err)
	}

}
