package rdbms

import (
	"database/sql"
	//"encoding/json"
	"fmt"
	"github.com/eaciit/cast"
	"github.com/eaciit/crowd"
	//"github.com/eaciit/database/base"
	"github.com/eaciit/dbox"
	"github.com/eaciit/errorlib"
	"github.com/eaciit/toolkit"
	"reflect"
	"strings"
	"time"
)

const (
	modQuery = "Query"
)

type Query struct {
	dbox.Query
	Sql        sql.DB
	usePooling bool
	DriverDB   string
}

func (q *Query) Session() sql.DB {
	q.usePooling = q.Config("pooling", false).(bool)
	// if q.Sql == nil {
	if q.usePooling {
		q.Sql = q.Connection().(*Connection).Sql
	} else {
		q.Sql = q.Connection().(*Connection).Sql
	}
	// }
	return q.Sql
}

func (q *Query) GetDriverDB() string {
	q.DriverDB = q.Connection().(*Connection).Drivername
	return q.DriverDB
}

func (q *Query) Close() {
	// if q.Sql != nil && q.usePooling == false {
	q.Sql.Close()
	// }
}

func (q *Query) Prepare() error {
	return nil
}

func StringValue(v interface{}, db string) string {
	var ret string
	switch v.(type) {
	case string:
		ret = fmt.Sprintf("%s", "'"+v.(string)+"'")
	case time.Time:
		t := v.(time.Time).UTC()
		if strings.Contains(db, "oracle") {
			ret = "to_date('" + t.Format("2006-01-02 15:04:05") + "','yyyy-mm-dd hh24:mi:ss')"
		} else {
			ret = "'" + t.Format("2006-01-02 15:04:05") + "'"
		}
	case int, int32, int64, uint, uint32, uint64:
		ret = fmt.Sprintf("%d", v.(int))
	case nil:
		ret = ""
	default:
		ret = fmt.Sprintf("%v", v)
		//-- do nothing
	}
	return ret
}

func (q *Query) Cursor(in toolkit.M) (dbox.ICursor, error) {
	var e error
	/*
		if q.Parts == nil {
			return nil, errorlib.Error(packageName, modQuery,
				"Cursor", fmt.Sprintf("No Query Parts"))
		}
	*/

	aggregate := false
	dbname := q.Connection().Info().Database
	session := q.Session()
	cursor := dbox.NewCursor(new(Cursor))
	cursor.(*Cursor).session = session
	// driverName := q.GetDriverDB()
	driverName := "oracle"

	/*
		parts will return E - map{interface{}}interface{}
		where each interface{} returned is slice of interfaces --> []interface{}
	*/
	parts := crowd.From(q.Parts()).Group(func(x interface{}) interface{} {
		qp := x.(*dbox.QueryPart)
		return qp.PartType
	}, nil).Data

	fromParts, hasFrom := parts[dbox.QueryPartFrom]
	procedureParts, hasProcedure := parts["procedure"]

	if hasFrom {
		tablename := ""
		tablename = fromParts.([]interface{})[0].(*dbox.QueryPart).Value.(string)

		skip := 0
		if skipParts, hasSkip := parts[dbox.QueryPartSkip]; hasSkip {
			skip = skipParts.([]interface{})[0].(*dbox.QueryPart).
				Value.(int)
		}

		take := 0
		if takeParts, has := parts[dbox.QueryPartTake]; has {
			take = takeParts.([]interface{})[0].(*dbox.QueryPart).
				Value.(int)
		}

		var fields toolkit.M
		selectParts, hasSelect := parts[dbox.QueryPartSelect]
		var attribute string
		incAtt := 0
		if hasSelect {
			fields = toolkit.M{}
			for _, sl := range selectParts.([]interface{}) {
				qp := sl.(*dbox.QueryPart)
				for _, fid := range qp.Value.([]string) {
					if incAtt == 0 {
						attribute = fid
					} else {
						attribute = attribute + "," + fid
					}
					incAtt++
					fields.Set(fid, 1)
				}
			}
		} else {
			_, hasUpdate := parts[dbox.QueryPartUpdate]
			_, hasInsert := parts[dbox.QueryPartInsert]
			_, hasDelete := parts[dbox.QueryPartDelete]
			_, hasSave := parts[dbox.QueryPartSave]

			if hasUpdate || hasInsert || hasDelete || hasSave {
				return nil, errorlib.Error(packageName, modQuery, "Cursor",
					"Valid operation for a cursor is select only")
			}
		}
		//fmt.Printf("Result: %s \n", toolkit.JsonString(fields))
		//fmt.Printf("Database:%s table:%s \n", dbname, tablename)
		var sort []string
		sortParts, hasSort := parts[dbox.QueryPartSelect]
		if hasSort {
			sort = []string{}
			for _, sl := range sortParts.([]interface{}) {
				qp := sl.(*dbox.QueryPart)
				for _, fid := range qp.Value.([]string) {
					sort = append(sort, fid)
				}
			}
		}

		//where := toolkit.M{}
		var where interface{}
		whereParts, hasWhere := parts[dbox.QueryPartWhere]
		if hasWhere {
			fb := q.Connection().Fb()
			for _, p := range whereParts.([]interface{}) {
				fs := p.(*dbox.QueryPart).Value.([]*dbox.Filter)
				for _, f := range fs { //get each element of 'Filter' Struct
					f.DriverName = driverName
					// if reflect.TypeOf(f.Value).Kind() == reflect.Slice {
					// 	fSlice := f.Value.([]interface{}) //get 'Value' field of 'Filter' Struct
					// 	for i, fv := range fSlice {       // get each value from [] interface
					// 		fSlice[i] = StringValue(fv, driverName)
					// 	}
					// 	f.Value = fSlice
					// } else {
					// 	f.Value = StringValue(f.Value, driverName)
					// }
					fb.AddFilter(f)
				}
			}
			where, e = fb.Build()
			if e != nil {
				return nil, errorlib.Error(packageName, modQuery, "Cursor",
					e.Error())
			} else {
				//fmt.Printf("Where: %s", toolkit.JsonString(where))
			}
			//where = iwhere.(toolkit.M)
		}

		if dbname != "" && tablename != "" && e != nil && skip == 0 && take == 0 && where == nil {

		}
		if !aggregate {
			QueryString := ""
			if attribute == "" {
				QueryString = "SELECT * FROM " + tablename
			} else {
				QueryString = "SELECT " + attribute + " FROM " + tablename
			}
			if cast.ToString(where) != "" {
				QueryString = QueryString + " WHERE " + cast.ToString(where)
			}
			cursor.(*Cursor).QueryString = QueryString
		}
	} else if hasProcedure {
		procCommand := procedureParts.([]interface{})[0].(*dbox.QueryPart).Value.(interface{})
		fmt.Println("Isi Proc command : ", procCommand)

		spName := procCommand.(toolkit.M)["name"].(string) + " "
		params := procCommand.(toolkit.M)["parms"]
		incParam := 0
		ProcStatement := ""

		if driverName == "mysql" {
			paramValue := ""
			paramName := ""

			for key, val := range params.(toolkit.M) {
				if incParam == 0 {
					paramValue = "('" + val.(string) + "'"
					paramName = key
				} else {
					paramValue += ",'" + val.(string) + "'"
				}
				incParam += 1
			}
			paramValue += ", " + paramName + ")"

			ProcStatement = "CALL " + spName + paramValue
		} else if driverName == "mssql" {
			paramstring := ""
			incParam := 0
			for key, val := range params.(toolkit.M) {
				if incParam == 0 {
					paramstring = key + " = '" + val.(string) + "'"
				} else {
					paramstring += ", " + key + " = '" + val.(string) + "'"
				}
				incParam += 1
			}
			paramstring += ";"

			ProcStatement = "EXECUTE " + spName + paramstring
		} else if driverName == "oracle" {
			paramstring := ""
			incParam := 0
			for key, val := range params.(toolkit.M) {
				if incParam == 0 {
					paramstring = key + " = '" + val.(string) + "'"
				} else {
					paramstring += ", " + key + " = '" + val.(string) + "'"
				}
				incParam += 1
			}
			paramstring += ";"

			ProcStatement = "EXECUTE " + spName + paramstring

		}

		cursor.(*Cursor).QueryString = ProcStatement

		fmt.Println("Proc Statement : ", ProcStatement)
	}
	return cursor, nil
}

func (q *Query) Exec(parm toolkit.M) error {
	var e error
	if parm == nil {
		parm = toolkit.M{}
	}
	// fmt.Println("Parameter Exec : ", parm)

	dbname := q.Connection().Info().Database
	tablename := ""

	if parm == nil {
		parm = toolkit.M{}
	}
	data := parm.Get("data", nil)

	// fmt.Println("Hasil ekstraksi Param : ", data)

	//========================EXTRACT FIELD, DATA AND FORMAT DATE=============================

	var attributes string
	var values string
	var setUpdate string

	if data != nil {

		var reflectValue = reflect.ValueOf(data)
		if reflectValue.Kind() == reflect.Ptr {
			reflectValue = reflectValue.Elem()
		}
		var reflectType = reflectValue.Type()

		for i := 0; i < reflectValue.NumField(); i++ {
			namaField := reflectType.Field(i).Name
			dataValues := reflectValue.Field(i).Interface()
			stringValues := StringValue(dataValues, q.GetDriverDB())
			if i == 0 {
				attributes = "(" + namaField
				values = "(" + stringValues
				setUpdate = namaField + " = " + stringValues
			} else {
				attributes += " , " + namaField
				values += " , " + stringValues
				setUpdate += " , " + namaField + " = " + stringValues
			}
		}
		attributes += ")"
		values += ")"
	}

	//=================================END OF EXTRACTION=======================================

	temp := ""
	parts := crowd.From(q.Parts()).Group(func(x interface{}) interface{} {
		qp := x.(*dbox.QueryPart)
		temp = toolkit.JsonString(qp)
		return qp.PartType
	}, nil).Data

	fromParts, hasFrom := parts[dbox.QueryPartFrom]
	if !hasFrom {

		return errorlib.Error(packageName, "Query", modQuery, "Invalid table name")
	}
	tablename = fromParts.([]interface{})[0].(*dbox.QueryPart).Value.(string)

	var where interface{}
	whereParts, hasWhere := parts[dbox.QueryPartWhere]
	if hasWhere {
		fb := q.Connection().Fb()
		for _, p := range whereParts.([]interface{}) {
			fs := p.(*dbox.QueryPart).Value.([]*dbox.Filter)
			for _, f := range fs {
				fb.AddFilter(f)
			}
		}
		where, e = fb.Build()
		if e != nil {

		} else {

		}

	}
	commandType := ""
	multi := false

	_, hasDelete := parts[dbox.QueryPartDelete]
	_, hasInsert := parts[dbox.QueryPartInsert]
	_, hasUpdate := parts[dbox.QueryPartUpdate]
	_, hasSave := parts[dbox.QueryPartSave]

	if hasDelete {
		commandType = dbox.QueryPartDelete
	} else if hasInsert {
		commandType = dbox.QueryPartInsert
	} else if hasUpdate {
		commandType = dbox.QueryPartUpdate
	} else if hasSave {
		commandType = dbox.QueryPartSave
	}

	if data == nil {
		//---
		multi = true
	} else {
		if where == nil {
			id := toolkit.Id(data)
			if id != nil {
				where = (toolkit.M{}).Set("_id", id)
			}
		} else {
			multi = true
		}
	}
	session := q.Session()

	if dbname != "" && tablename != "" && multi == true {

	}
	if commandType == dbox.QueryPartInsert {

	} else if commandType == dbox.QueryPartUpdate {
		statement := "UPDATE " + tablename + " SET " + setUpdate + " WHERE " + cast.ToString(where)
		fmt.Println("Update Statement : ", statement)
		_, e = session.Exec(statement)
		if e != nil {
			fmt.Println(e.Error())
		}

	} else if commandType == dbox.QueryPartDelete {
		if where == nil {
			statement := "DELETE FROM " + tablename
			fmt.Println(statement)
			_, e = session.Exec(statement)
			if e != nil {
				fmt.Println(e.Error())
			}
		} else {
			statement := "DELETE FROM " + tablename + " where " + cast.ToString(where)
			fmt.Println(statement)
			_, e = session.Exec(statement)
			if e != nil {
				fmt.Println(e.Error())
			}
		}

	} else if commandType == dbox.QueryPartSave {

		statement := "INSERT INTO " + tablename + " " + attributes + " VALUES " + values
		fmt.Println("Insert Statement : ", statement)
		_, e = session.Exec(statement)
		if e != nil {
			fmt.Println(e.Error())
		}
	}
	if e != nil {
		return errorlib.Error(packageName, modQuery+".Exec", commandType, e.Error())
	}
	return nil
}
