package actions

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"io/ioutil"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/stretchr/testify/assert"
)

func TestSimpleCorrelate(t *testing.T) {
	openDatabaseList = openDatabaseListT{m: make(map[string]*sql.DB)}

	tempdir, err := ioutil.TempDir("", "correlate_test")
	if err != nil {
		logp.Err("Error creating temporary directory")
		t.Fatal(err)
	}
	defer os.RemoveAll(tempdir)
	databaseFile := filepath.Join(tempdir, "mydb.db.sqlite")
	conf, err := common.NewConfigFrom(map[string]interface{}{"field_key": "session_id",
		"database_name": "mydb",
		"database":      databaseFile,
		"copied_fields": []string{"ipAddress", "loginTime"}})

	if err != nil {
		logp.Err("Error initializing config ")
		t.Fatal(err)
	}

	corrCreate, err := newCorrelateCreate(*conf)

	defer corrCreate.(correlateCreate).Close()

	confUse, err := common.NewConfigFrom(map[string]interface{}{"field_key": "session_id", "database_name": "mydb"})

	corrUse, err := newCorrelateUse(*confUse)

	confDelete, err := common.NewConfigFrom(map[string]interface{}{"field_key": "session_id", "database_name": "mydb"})

	corrDelete, err := newCorrelateDelete(*confDelete)

	event1 := common.MapStr{
		"session_id": "{28d98a23-b522-4824-b1b1-7b4d2bb2488a}",
		"ipAddress":  "10.1.1.30",
		"loginTime":  "05-10-2001T12:02:01.03",
	}
	event1Copy := event1.Clone()

	event2 := common.MapStr{
		"session_id":  "{28d98a23-b522-4824-b1b1-7b4d2bb2488a}",
		"processName": "cmd.exe",
	}

	event1p, err := corrCreate.Run(event1)
	assert.Nil(t, err)

	event2p, err := corrUse.Run(event2)
	assert.Nil(t, err)

	event2pExpected := common.MapStr{
		"session_id":  "{28d98a23-b522-4824-b1b1-7b4d2bb2488a}",
		"processName": "cmd.exe",
		"ipAddress":   "10.1.1.30",
		"loginTime":   "05-10-2001T12:02:01.03",
	}

	assert.Equal(t, event1Copy, event1p)
	assert.Equal(t, event2pExpected, event2p)

	event3 := common.MapStr{
		"session_id": "{28d98a23-b522-4824-b1b1-7b4d2bb2488a}",
	}

	event3p, err := corrDelete.Run(event3)
	assert.Nil(t, err)

	assert.Equal(t, event3, event3p)

	event2AfterDel := common.MapStr{
		"session_id":  "{28d98a23-b522-4824-b1b1-7b4d2bb2488a}",
		"processName": "notepad.exe",
	}

	event2AfterDelP, err := corrUse.Run(event2AfterDel)
	event2AfterDelPCopy := event2AfterDelP.Clone()
	assert.Nil(t, err)
	assert.Equal(t, event2AfterDelPCopy, event2AfterDelP)

}
