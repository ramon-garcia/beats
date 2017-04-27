package actions

import (
	"testing"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/stretchr/testify/assert"
)

func TestParseHexadecimal(t *testing.T) {
	input := common.MapStr{
		"ticks": "0xdead",
	}
	script := `event.ticks = parseInt(event.ticks.replace(/^0x/, ''), 16);`
	out, err := testJavascript(t, input, script)
	assert.Nil(t, err)

	expected := common.MapStr{
		"ticks": int64(0xdead),
	}

	assert.Equal(t, expected, out)
}

func TestNested(t *testing.T) {
	input := common.MapStr{
		"event_id": int64(11),
		"event_data": common.MapStr{
			"image": "C:\\Program Files (x86)\\Mozilla Firefox\\firefox.exe",
		},
	}
	script := `event.exec = event.event_data.image;`
	out, err := testJavascript(t, input, script)
	assert.Nil(t, err)

	expected := common.MapStr{
		"event_id": int64(11),
		"event_data": common.MapStr{
			"image": "C:\\Program Files (x86)\\Mozilla Firefox\\firefox.exe",
		},
		"exec": "C:\\Program Files (x86)\\Mozilla Firefox\\firefox.exe",
	}

	assert.Equal(t, expected, out)
}

func testJavascript(t *testing.T, event common.MapStr, script string) (common.MapStr, error) {
	config, _ := common.NewConfigFrom(map[string]interface{}{
		"script": script,
	})
	jsProc, err := newJavascript(*config)
	if err != nil {
		logp.Err("Error creating Javascript processor")
		t.Fatal(err)
	}
	result, ok := jsProc.Run(event)
	return result, ok
}
