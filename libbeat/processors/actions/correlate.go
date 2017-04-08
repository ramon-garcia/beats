package actions

import (
	"database/sql"
	"fmt"

	"sync"

	"strings"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors"
	"github.com/mattn/go-sqlite3"
)

type correlateCreate struct {
	fieldKey         string
	databaseFileName string
	database         *sql.DB
	copiedFields     []string
	insertStmt       *sql.Stmt
}

var openDatabaseList = struct {
	sync.RWMutex
	m map[string]*sql.DB
}{m: make(map[string]*sql.DB)}

func init() {
	processors.RegisterPlugin("correlate_create",
		configChecked(newCorrelateCreate,
			requireFields("field_key", "database_name", "copied_fields"),
			allowedFields("field_key", "database", "database_name", "copied_fields", "when")))
	processors.RegisterPlugin("correlate_use",
		configChecked(newCorrelateUse,
			requireFields("field_key", "database_name"),
			allowedFields("field_key", "database_name", "nested_field", "when")))
}

func newCorrelateCreate(c common.Config) (processors.Processor, error) {
	type config struct {
		FieldKey     string   `config:"field_key"`
		Database     string   `config:"database"`
		DatabaseName string   `config:"database_name"`
		CopiedFields []string `config:"copied_fields"`
	}

	var myconfig config
	err := c.Unpack(&myconfig)
	if err != nil {
		logp.Warn("Error unpacking config for correlateCreate")
		return nil, fmt.Errorf("fail to unpack the grok configuration: %s", err)
	}
	var database *sql.DB
	if myconfig.DatabaseName != "" {
		database, err = sql.Open("sqlite3", myconfig.Database)
		defer func() {
			if database != nil {
				database.Close()
			}
		}()
		if err != nil {
			logp.Warn("Error opening database", myconfig.Database, err)
			return nil, fmt.Errorf("Error opening database %s: %s", myconfig.Database, err)
		}
	}
	err = func() error {
		openDatabaseList.Lock()
		defer openDatabaseList.Unlock()
		oldDatabase, ok := openDatabaseList.m[myconfig.DatabaseName]
		if ok {
			if database == nil {
				database = oldDatabase
				return nil
			}
			return fmt.Errorf("Database name '%s' already exists ", myconfig.DatabaseName)
		}
		openDatabaseList.m[myconfig.DatabaseName] = database
		return nil
	}()
	if err != nil {
		return nil, err
	}
	createTableStmt := "CREATE TABLE correlated (" + sqlQuoteColumn("_beat_corr_key") + " VARCHAR(256) PRIMARY KEY"
	for _, field := range myconfig.CopiedFields {
		createTableStmt = createTableStmt + ", " + sqlQuoteColumn(field) + " VARCHAR(256)"
	}
	createTableStmt = createTableStmt + ");"

	_, err = database.Exec(createTableStmt)
	if err != nil {
		errsq := err.(sqlite3.Error)
		if errsq.Code == sqlite3.ErrError && errsq.ExtendedCode == sqlite3.ErrNoExtended(1) && strings.Contains(errsq.Error(), "already exists") {
			logp.Info("table creation omitted, already exists")
			// TODO: check that the table is compatible. Otherwise, drop it
		} else {
			logp.Warn("Error creating table", err)
			return nil, fmt.Errorf("Error creating table %s", err)
		}
	}

	insertStmt := "INSERT INTO correlated (" + sqlQuoteColumn("_beat_corr_key")
	for _, field := range myconfig.CopiedFields {
		insertStmt = insertStmt + ", " + sqlQuoteColumn(field)
	}
	insertStmt = insertStmt + ") VALUES (?"
	for _ = range myconfig.CopiedFields {
		insertStmt = insertStmt + ", ?"
	}
	insertStmt = insertStmt + ")"
	insertStmtDb, err := database.Prepare(insertStmt)
	if err != nil {
		logp.Warn("Error preparing insert statement", insertStmt, err)
		return nil, fmt.Errorf("Error preparing insert statement %s: %s", insertStmt, err)
	}

	// Trick to avoid closing the database if there has been no error (assign to nil so that the defer does not close it)
	databaseToUse := database
	database = nil
	return correlateCreate{fieldKey: myconfig.FieldKey,
		databaseFileName: myconfig.Database,
		database:         databaseToUse,
		copiedFields:     myconfig.CopiedFields,
		insertStmt:       insertStmtDb}, nil
}

func sqlQuoteColumn(column string) string {
	// FIXME
	return column
}

func (ccor correlateCreate) Run(event common.MapStr) (common.MapStr, error) {
	key := event[ccor.fieldKey].(string)
	insertedValues := make([]interface{}, len(ccor.copiedFields)+1)
	insertedValues[0] = key
	for i := 1; i < len(ccor.copiedFields)+1; i++ {
		insertedValues[i] = event[ccor.copiedFields[i-1]].(string)
	}
	_, err := ccor.insertStmt.Exec(insertedValues...)
	if err != nil {
		logp.Warn("Error inserting value for key %s to correlation table: %s", key, err)
	}
	return event, nil
}

func (ccor correlateCreate) String() string {
	return "correlateCreate key=" + ccor.fieldKey + " database=" + ccor.databaseFileName + " copiedFields=" + strings.Join(ccor.copiedFields, ", ")
}

func (ccor correlateCreate) Close() error {
	ccor.insertStmt.Close()
	return ccor.database.Close()
}

type correlateUse struct {
	fieldKey     string
	database     *sql.DB
	databaseName string
	nestedField  string
	queryStmt    *sql.Stmt
}

func newCorrelateUse(c common.Config) (processors.Processor, error) {
	var config struct {
		FieldKey     string `config:"field_key"`
		DatabaseName string `config:"database_name"`
		NestedField  string `config:"nested_field"`
	}
	err := c.Unpack(&config)
	if err != nil {
		logp.Warn("Error unpacking config for correlateUse")
		return nil, fmt.Errorf("fail to unpack the correlateUse configuration: %s", err)
	}
	database, ok := func() (*sql.DB, bool) {
		openDatabaseList.RLock()
		defer openDatabaseList.RUnlock()
		db, ok := openDatabaseList.m[config.DatabaseName]
		return db, ok
	}()
	if !ok {
		logp.Warn("Error database not declared", config.DatabaseName)
		return nil, fmt.Errorf("Error database not declared %s", config.DatabaseName)
	}
	queryStmtText := "SELECT * from correlated WHERE _beat_corr_key = ?"
	queryStmt, err := database.Prepare(queryStmtText)
	if err != nil {
		logp.Warn("Error preparing query statment")
		return nil, fmt.Errorf("Error preparing query statment: \"%s\" %s", queryStmtText, err)
	}
	//corrColumnsQ, err := database.Query("PRAGMA table_info(correlated)")
	//corrColumnsQ.

	return correlateUse{fieldKey: config.FieldKey, database: database, databaseName: config.DatabaseName, nestedField: config.NestedField, queryStmt: queryStmt}, nil
}

func (ccor correlateUse) Run(event common.MapStr) (common.MapStr, error) {
	fieldKey, ok := event[ccor.fieldKey]
	if !ok {
		return event, nil
	}
	rows, err := ccor.queryStmt.Query(fieldKey)
	if err != nil {
		logp.Info("correlate_use database %s : problem with query %s", ccor.databaseName, err)
		return nil, err
	}
	if !rows.Next() {
		logp.Info("correlate_use database %s : key %s not found", ccor.databaseName, event[ccor.fieldKey])
		return event, nil
	}
	if rows.Err() != nil {
		logp.Err("correlate_use: error querying database %s key %s error %s", ccor.databaseName, event[ccor.fieldKey], rows.Err())
		return event, nil
	}
	columns, err := rows.Columns()
	if err != nil {
		logp.Err("correlate_use: error querying database %s key %s error %s", ccor.databaseName, event[ccor.fieldKey], rows.Err())
		return nil, err
	}
	correlatedData := make([]string, len(columns))
	correlatedDataArg := make([]interface{}, len(columns))
	for i := range correlatedData {
		correlatedDataArg[i] = &correlatedData[i]
	}
	err = rows.Scan(correlatedDataArg...)
	if err != nil {
		logp.Err("correlate_use: error reading query reply %s key %s error %s", ccor.databaseName, event[ccor.fieldKey], rows.Err())
		return event, nil
	}
	for i, data := range correlatedData {
		name := columns[i]
		if name == "_beat_corr_key" {
			continue
		}
		if ccor.nestedField == "" {
			event[name] = data
		} else {
			nestedFieldR, ok := event[ccor.nestedField]
			nestedField := nestedFieldR.(common.MapStr)
			if !ok {
				nestedField = make(common.MapStr)
				event[ccor.nestedField] = nestedField
			}
			nestedField[name] = data
		}
	}
	return event, nil
}

func (ccor correlateUse) String() string {
	return "correlateUse database=" + ccor.databaseName + " field_key=" + ccor.fieldKey
}
