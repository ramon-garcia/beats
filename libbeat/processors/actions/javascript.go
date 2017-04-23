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
			allowedFields("field", "when")))
}

func newJavascript(c common.Config) (processors.Processor, error) {
	var config struct {
		script string `config:"script"`
	}
	err := c.Unpack(&config)
	if err != nil {
		logp.Warn("Error unpacking config for javascript")
		return nil, fmt.Errorf("fail to unpack the javascript configuration: %s", err)
	}
	vm := otto.New()
	scriptCompiled, err := vm.Compile("", config.script)
	if err != nil {
		logp.Warn("Error compiling javascript script")
		return nil, fmt.Errorf("compiling javascript script: %s", err)
	}
	return javascript{vm, scriptCompiled, config.script}, nil
}

func (j javascript) Run(event common.MapStr) (common.MapStr, error) {
	object, _ := j.vm.Object(`{}`)
	for k, v := range event {
		object.Set(k, v)
	}
	j.vm.Set("event", object)
	j.vm.Run(j.script)
	result := common.MapStr{}
	for _, k := range object.Keys() {
		value, _ := object.Get(k)
		result[k], _ = value.ToString()
	}
	return result, nil
}

func (j javascript) String() string {
	return "javascript script=<![CDATA[" + j.script + "]]>"
}
