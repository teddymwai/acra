package encryptor

import (
	"github.com/cossacklabs/acra/sqlparser"
	"testing"
)

func TestGetFirstTableWithoutAlias(t *testing.T) {
	type testcase struct {
		SQL   string
		Table string
		Error error
	}
	testcases := []testcase{
		{SQL: `select * from table1, table2 t2`, Table: "table1"},
		{SQL: `select * from table2 t2, table1`, Table: "table1"},
		{SQL: `select * from table2 t2, table1, table3 as t3`, Table: "table1"},
		{SQL: `select * from table2 t2, table1 t1, table3 as t3`, Error: errNotFoundtable},
		{SQL: `select * from table1 join table2 join table3 join table4`, Table: "table1"},
		{SQL: `select * from table1 t2 join table2 `, Error: errNotFoundtable},
	}

	parser := sqlparser.New(sqlparser.ModeStrict)
	for _, tcase := range testcases {
		parsed, err := parser.Parse(tcase.SQL)
		if err != nil {
			t.Fatal(err)
		}
		selectExpr, ok := parsed.(*sqlparser.Select)
		if !ok {
			t.Fatal("Test cases should contain only Select queries")
		}
		tableName, err := getFirstTableWithoutAlias(selectExpr.From)
		if err != tcase.Error {
			t.Fatal(err)
		}
		// if expected error then we don't need to compare table name
		if tcase.Error != nil {
			continue
		}
		if tableName != tcase.Table {
			t.Fatal("Parsed incorrect table name without alias")
		}
	}
}

func TestMapColumnsToAliases(t *testing.T) {
	parser := sqlparser.New(sqlparser.ModeStrict)
	t.Run("With enumeration fields query", func(t *testing.T) {
		query := `
select t1.col1, t1.col2, t2.col1, t2.col2, t3.col1, t4.col4, col5, table6.col6
from table5, table6
inner join (select col1, col22 as col2, col3 from table1) as t1
inner join (select t1.col1, t2.col3 col2, t1.col3 from table1 t1 inner join table2 t2 on t1.col1=t2.col1) as t2 on t2.col1=t1.col1
inner join table3 t3 on t3.col1=t1.col1
inner join table4 as t4 on t4.col4=t1.col4
inner join table6 on table6.col1=t1.col1
`
		expectedValues := []columnInfo{
			// column's alias is subquery alias with column and table without aliases in subquery
			{Alias: "t1", Table: "table1", Name: "col1"},
			// column's alias is subquery alias with column with AS expression and table without alias
			{Alias: "t1", Table: "table1", Name: "col22"},
			// column's alias is subquery alias and column name has alias in subquery to table with alias
			{Alias: "t2", Table: "table1", Name: "col1"},
			// column's alias is subquery alias and column name has alias in subquery to joined table with alias
			{Alias: "t2", Table: "table2", Name: "col3"},
			// column's alias is alias of joined table
			{Alias: "t3", Table: "table3", Name: "col1"},
			// column's alias is alias of joined table with AS expression
			{Alias: "t4", Table: "table4", Name: "col4"},
			// column without alias of table in FROM expression
			{Table: "table5", Name: "col5", Alias: "table5"},
			// column with alias as table name in FROM expression
			{Table: "table6", Name: "col6", Alias: "table6"},
		}
		parsed, err := parser.Parse(query)
		if err != nil {
			t.Fatal(err)
		}
		selectExpr, ok := parsed.(*sqlparser.Select)
		if !ok {
			t.Fatal("Test query should be Select expression")
		}
		columns, err := mapColumnsToAliases(selectExpr)
		if err != nil {
			t.Fatal(err)
		}
		if len(columns) != len(expectedValues) {
			t.Fatal("Returned incorrect length of values")
		}

		for i, column := range columns {
			if column == nil {
				t.Fatalf("[%d] Column info not found", i)
			}

			if *column != expectedValues[i] {
				t.Fatalf("[%d] Column info is not equal to expected - %+v, actual - %+v", i, expectedValues[i], *column)
			}
		}
	})
	t.Run("Join enumeration fields query", func(t *testing.T) {
		query := `select table1.number, from_number, to_number, type, amount, created_date 
from table1 join table2 as t2 on from_number = t2.number or to_number = t2.number join users as u on t2.user_id = u.id`

		parsed, err := parser.Parse(query)
		if err != nil {
			t.Fatal(err)
		}
		selectExpr, ok := parsed.(*sqlparser.Select)
		if !ok {
			t.Fatal("Test query should be Select expression")
		}

		expectedValues := []columnInfo{
			{Alias: "table1", Table: "table1", Name: "number"},
			{Alias: "table1", Table: "table1", Name: "from_number"},
			{Alias: "table1", Table: "table1", Name: "to_number"},
			{Alias: "table1", Table: "table1", Name: "type"},
			{Alias: "table1", Table: "table1", Name: "amount"},
			{Alias: "table1", Table: "table1", Name: "created_date"},
		}

		columns, err := mapColumnsToAliases(selectExpr)
		if err != nil {
			t.Fatal(err)
		}

		if len(columns) != len(expectedValues) {
			t.Fatal("Returned incorrect length of values")
		}

		for i, column := range columns {
			if column == nil {
				t.Fatalf("[%d] Column info not found", i)
			}

			if *column != expectedValues[i] {
				t.Fatalf("[%d] Column info is not equal to expected - %+v, actual - %+v", i, expectedValues[i], *column)
			}
		}
	})
	t.Run("Join enumeration asterisk query", func(t *testing.T) {
		queries := []string{
			`select *  from  test_table join test_table2 join test_table3 t3 on t2.id = t3.id join test_table4 t4 on t3.id = t4.id`,
			`select t2.*, t3.*  from  test_table join test_table2 t2 join test_table3 t3 on t2.id = t3.id join test_table4 t4 on t3.id = t4.id`,
			`select t2.*, t3.*, *  from  test_table join test_table2 t2 join test_table3 t3 on t2.id = t3.id join test_table4 t4 on t3.id = t4.id`,
		}

		expectedValues := [][]columnInfo{
			{
				{Alias: allColumnsName, Table: "test_table", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table2", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table3", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table4", Name: allColumnsName},
			},
			{
				{Alias: allColumnsName, Table: "test_table2", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table3", Name: allColumnsName},
			},
			{
				{Alias: allColumnsName, Table: "test_table2", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table3", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table2", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table3", Name: allColumnsName},
				{Alias: allColumnsName, Table: "test_table4", Name: allColumnsName},
			},
		}

		for i, query := range queries {
			parsed, err := parser.Parse(query)
			if err != nil {
				t.Fatal(err)
			}
			selectExpr, ok := parsed.(*sqlparser.Select)
			if !ok {
				t.Fatal("Test query should be Select expression")
			}

			columns, err := mapColumnsToAliases(selectExpr)
			if err != nil {
				t.Fatal(err)
			}

			if len(columns) != len(expectedValues[i]) {
				t.Fatal("Returned incorrect length of values")
			}

			for c, column := range columns {
				if column == nil {
					t.Fatalf("[%d] Column info not found", i)
				}

				if *column != expectedValues[i][c] {
					t.Fatalf("[%d] Column info is not equal to expected - %+v, actual - %+v", i, expectedValues[i][c], *column)
				}
			}
		}
	})

	t.Run("With asterisk query", func(t *testing.T) {
		query := `select * from test_table`

		parsed, err := parser.Parse(query)
		if err != nil {
			t.Fatal(err)
		}
		selectExpr, ok := parsed.(*sqlparser.Select)
		if !ok {
			t.Fatal("Test query should be Select expression")
		}

		expectedValue := columnInfo{Alias: "*", Table: "test_table", Name: "*"}

		columns, err := mapColumnsToAliases(selectExpr)
		if err != nil {
			t.Fatal(err)
		}

		if len(columns) != 1 {
			t.Fatal("Returned incorrect length of values")
		}

		column := columns[0]

		if column == nil {
			t.Fatal("Column info not found")
		}

		if *column != expectedValue {
			t.Fatalf("Column info is not equal to expected - %+v, actual - %+v", expectedValue, *column)
		}
	})
	t.Run("With table asterisk query", func(t *testing.T) {
		query := `select t1.*, t2.* from test_table t1, test_table t2`

		parsed, err := parser.Parse(query)
		if err != nil {
			t.Fatal(err)
		}
		selectExpr, ok := parsed.(*sqlparser.Select)
		if !ok {
			t.Fatal("Test query should be Select expression")
		}

		expectedValue := []columnInfo{
			{Alias: allColumnsName, Table: "test_table", Name: allColumnsName},
			{Alias: allColumnsName, Table: "test_table", Name: allColumnsName},
		}

		columns, err := mapColumnsToAliases(selectExpr)
		if err != nil {
			t.Fatal(err)
		}

		if len(columns) != len(expectedValue) {
			t.Fatal("Returned incorrect length of values")
		}

		for i, expectedColumn := range expectedValue {
			if columns[i].Name != expectedColumn.Name || columns[i].Alias != expectedColumn.Alias || columns[i].Table != expectedColumn.Table {
				t.Fatalf("Column info is not equal to expected - %+v, actual - %+v", expectedValue, columns[i])
			}
		}
	})
}
