package actions

import (
	"fmt"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors"
	"github.com/robertkrimen/otto"
)

type javascript struct {
	vm             *otto.Otto
	scriptCompiled *otto.Script
	script         string
}

func init() {
	processors.RegisterPlugin("javascript",
		configChecked(newJavascript,
			requireFields("script"),
			allowedFields("script", "when")))
}

func newJavascript(c common.Config) (processors.Processor, error) {
	var config struct {
		Script string `config:"script"`
	}
	err := c.Unpack(&config)
	if err != nil {
		logp.Warn("Error unpacking config for javascript")
		return nil, fmt.Errorf("fail to unpack the javascript configuration: %s", err)
	}
	vm := otto.New()
	scriptCompiled, err := vm.Compile("", config.Script)
	if err != nil {
		logp.Warn("Error compiling javascript script")
		return nil, fmt.Errorf("compiling javascript script: %s", err)
	}
	return javascript{vm, scriptCompiled, config.Script}, nil
}

func (j javascript) Run(event common.MapStr) (common.MapStr, error) {
	j.vm.Set("event", event)
	j.vm.Run(j.script)
	result, err := j.vm.Get("event")
	if err != nil {
		return nil, err
	}
	resultGo, err := result.Export()
	return resultGo.(common.MapStr), err
}

func (j javascript) String() string {
	return "javascript script=<![CDATA[" + j.script + "]]>"
}
