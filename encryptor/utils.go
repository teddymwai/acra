/*
Copyright 2019, Cossack Labs Limited

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package encryptor

import (
	"errors"
	"github.com/cossacklabs/acra/sqlparser"
)

var errNotFoundtable = errors.New("not found table for alias")
var errNotSupported = errors.New("not supported type of sql node")

type columnInfo struct {
	Name  string
	Table string
	Alias string
}

var errEmptyTableExprs = errors.New("empty table exprs")

// parseJoinTablesInfo recursively read and save sql join structure info, aliases map is used to save association between tables and its aliases,
// tables slice is used to collect certain order of tables (saved in reverse order of declaration).
// JoinTableExpr structure represent a recursive tree where RightExpr and LeftExpr are corresponded leaf node
// recursive processing starts from RightExpr leaf to the LeftExpr one, and when cast LeftExpr to AliasedTableExpr is successful
// it means that we reach last leaf in the tree.
func parseJoinTablesInfo(joinExp *sqlparser.JoinTableExpr, tables *[]string, aliases map[string]string) bool {
	aliased, ok := joinExp.LeftExpr.(*sqlparser.AliasedTableExpr)
	if ok {
		// here we reach the last leaf in the JoinTableExpr recursive tree, processing SHOULD be stopped in this block.
		// and we should process remaining RightExpr and LeftExpr leafs more before exit.
		name, alias, ok := getRightJoinTableInfo(joinExp)
		if !ok {
			return false
		}
		*tables = append(*tables, name)
		aliases[alias] = name

		tableName, ok := aliased.Expr.(sqlparser.TableName)
		if !ok {
			return false
		}

		alias = aliased.As.RawValue()
		if aliased.As.RawValue() == "" {
			alias = tableName.Name.RawValue()
		}

		*tables = append(*tables, tableName.Name.RawValue())
		aliases[alias] = tableName.Name.RawValue()
		return true
	}

	name, alias, ok := getRightJoinTableInfo(joinExp)
	if !ok {
		return false
	}
	*tables = append(*tables, name)
	aliases[alias] = name

	joinExp, ok = joinExp.LeftExpr.(*sqlparser.JoinTableExpr)
	if !ok {
		return false
	}

	return parseJoinTablesInfo(joinExp, tables, aliases)
}

// getRightJoinTableInfo return tableName and its alias for right join table
func getRightJoinTableInfo(joinExp *sqlparser.JoinTableExpr) (string, string, bool) {
	rAliased, ok := joinExp.RightExpr.(*sqlparser.AliasedTableExpr)
	if !ok {
		return "", "", false
	}

	tableName, ok := rAliased.Expr.(sqlparser.TableName)
	if !ok {
		return "", "", false
	}

	alias := rAliased.As.RawValue()
	if rAliased.As.RawValue() == "" {
		alias = tableName.Name.RawValue()
	}

	return tableName.Name.RawValue(), alias, true
}

// getJoinFirstTableWithoutAlias recursively process JoinTableExpr tree until it reaches the first table in JOIN declarations
// used to handle queries like this `select table1.column1, column2, column3 from table1 join table2 as t2` and match column2 to table1
func getJoinFirstTableWithoutAlias(joinExp *sqlparser.JoinTableExpr) (string, bool) {
	aliased, ok := joinExp.LeftExpr.(*sqlparser.AliasedTableExpr)
	if ok {
		return getNonAliasedName(aliased)
	}

	joinExp, ok = joinExp.LeftExpr.(*sqlparser.JoinTableExpr)
	if !ok {
		return "", false
	}
	return getJoinFirstTableWithoutAlias(joinExp)
}

// getFirstTableWithoutAlias search table name from "FROM" expression which has not any alias
// if more than one table specified without alias then return errNotFoundTable
func getFirstTableWithoutAlias(fromExpr sqlparser.TableExprs) (string, error) {
	if len(fromExpr) == 0 {
		return "", errEmptyTableExprs
	}

	if joinExp, ok := fromExpr[0].(*sqlparser.JoinTableExpr); ok {
		tableName, ok := getJoinFirstTableWithoutAlias(joinExp)
		if !ok {
			return "", errNotFoundtable
		}
		return tableName, nil
	}

	var name string
	for _, tblExpr := range fromExpr {
		aliased, ok := tblExpr.(*sqlparser.AliasedTableExpr)
		if !ok {
			continue
		}
		tName, ok := getNonAliasedName(aliased)
		if !ok {
			continue
		}
		if name != "" {
			return "", errors.New("more than 1 table without alias")
		}
		name = tName
	}
	if name == "" {
		return "", errNotFoundtable
	}
	return name, nil
}

func getNonAliasedName(aliased *sqlparser.AliasedTableExpr) (string, bool) {
	if !aliased.As.IsEmpty() {
		return "", false
	}
	tableName, ok := aliased.Expr.(sqlparser.TableName)
	if !ok {
		return "", false
	}
	return tableName.Name.RawValue(), true
}

func getTableNameWithoutAliases(expr sqlparser.TableExpr) (string, error) {
	aliased, ok := expr.(*sqlparser.AliasedTableExpr)
	if !ok {
		return "", errNotFoundtable
	}
	tableName, ok := aliased.Expr.(sqlparser.TableName)
	if !ok {
		return "", errNotFoundtable
	}
	return tableName.Name.RawValue(), nil
}

func findTableName(alias, columnName string, expr sqlparser.SQLNode) (columnInfo, error) {
	switch val := expr.(type) {
	case sqlparser.TableExprs:
		// FROM table1, table2, join ....
		// search through list of tables by specific type of sql node (AliasedTableExpr, Join, ...)
		for _, tblExpr := range val {
			result, err := findTableName(alias, columnName, tblExpr)
			if err == nil {
				return result, nil
			}
		}
		return columnInfo{}, errNotFoundtable
	case sqlparser.TableName:
		// table1, should be equal to end alias value
		if alias == val.Name.RawValue() {
			return columnInfo{Name: columnName, Table: alias}, nil
		}
		return columnInfo{}, errNotFoundtable
	case *sqlparser.AliasedTableExpr:
		if val.As.IsEmpty() {
			return findTableName(alias, columnName, val.Expr)
		}
		if val.As.RawValue() == alias {
			if tblName, ok := val.Expr.(sqlparser.TableName); ok {
				return findTableName(tblName.Name.RawValue(), columnName, val.Expr)
			}
			return findTableName("", columnName, val.Expr)
		}
	case *sqlparser.Subquery:
		return findTableName(alias, columnName, val.Select)
	case *sqlparser.Select:
		for _, expr := range val.SelectExprs {
			if aliasExpr, ok := expr.(*sqlparser.AliasedExpr); ok {
				if aliasExpr.As.IsEmpty() {
					// select t1.col1
					switch aliasVal := aliasExpr.Expr.(type) {
					case *sqlparser.ColName:
						if aliasVal.Qualifier.IsEmpty() {
							// select col1
							if aliasVal.Name.EqualString(columnName) {
								// find first table in FROM list
								firstTable, err := getFirstTableWithoutAlias(val.From)
								if err != nil {
									continue
								}
								return columnInfo{Name: columnName, Table: firstTable}, nil
							}
						} else {
							// t1.col1 == col1 so we should find source name of t1.
							if aliasVal.Name.EqualString(columnName) {
								return findTableName(aliasVal.Qualifier.Name.RawValue(), aliasVal.Name.String(), val.From)
							}
						}
						continue
					}
				} else if aliasExpr.As.EqualString(alias) || (alias == "" && aliasExpr.As.EqualString(columnName)) {
					// select t1.col1 as columnName
					switch aliasVal := aliasExpr.Expr.(type) {
					case *sqlparser.ColName:
						if aliasVal.Qualifier.Name.RawValue() == "" {
							firstTable, err := getFirstTableWithoutAlias(val.From)
							if err != nil {
								return columnInfo{}, err
							}
							return findTableName(firstTable, aliasVal.Name.String(), val.From)
						}
						return findTableName(aliasVal.Qualifier.Name.RawValue(), aliasVal.Name.String(), val.From)
					}
				}
			}
		}
	case *sqlparser.JoinTableExpr:
		result, err := findTableName(alias, columnName, val.LeftExpr)
		if err == errNotFoundtable {
			return findTableName(alias, columnName, val.RightExpr)
		}
		return result, err
	case *sqlparser.Union:
		// may be different pairs of table + column at same position of result row
		return columnInfo{}, errNotSupported
	case *sqlparser.ParenSelect:
		return findTableName(alias, columnName, val.Select)
	}
	return columnInfo{}, errNotFoundtable
}

func mapColumnsToAliases(selectQuery *sqlparser.Select) ([]*columnInfo, error) {
	out := make([]*columnInfo, 0, len(selectQuery.SelectExprs))
	var joinTables []string
	var joinAliases map[string]string

	if joinExp, ok := selectQuery.From[0].(*sqlparser.JoinTableExpr); ok {
		joinTables = make([]string, 0)
		joinAliases = make(map[string]string)

		if ok := parseJoinTablesInfo(joinExp, &joinTables, joinAliases); !ok {
			return nil, errUnsupportedExpression
		}
	}

	for _, expr := range selectQuery.SelectExprs {
		aliased, ok := expr.(*sqlparser.AliasedExpr)
		if ok {
			colName, ok := aliased.Expr.(*sqlparser.ColName)
			if ok {
				if colName.Qualifier.Name.IsEmpty() {
					firstTable, err := getFirstTableWithoutAlias(selectQuery.From)
					if err != nil {
						out = append(out, nil)
						continue
					}
					info, err := findTableName(firstTable, colName.Name.String(), selectQuery.From)
					if err == nil {
						info.Alias = firstTable
						out = append(out, &info)
						continue
					}
				} else {
					info, err := findTableName(colName.Qualifier.Name.RawValue(), colName.Name.String(), selectQuery.From)
					if err == nil {
						info.Alias = colName.Qualifier.Name.RawValue()
						out = append(out, &info)
						continue
					}
				}
			}
		}
		starExpr, ok := expr.(*sqlparser.StarExpr)
		if ok {
			if len(joinTables) > 0 {
				if !starExpr.TableName.Name.IsEmpty() {
					joinTable, ok := joinAliases[starExpr.TableName.Name.String()]
					if !ok {
						return nil, errUnsupportedExpression
					}
					out = append(out, &columnInfo{Table: joinTable, Name: allColumnsName, Alias: allColumnsName})
					continue
				}

				for i := len(joinTables) - 1; i >= 0; i-- {
					out = append(out, &columnInfo{Table: joinTables[i], Name: allColumnsName, Alias: allColumnsName})
				}
				continue
			}

			tableName, err := getFirstTableWithoutAlias(selectQuery.From)
			if err == nil {
				out = append(out, &columnInfo{Table: tableName, Name: allColumnsName, Alias: allColumnsName})
			} else {
				if len(selectQuery.From) == 1 {
					tableNameStr, err := getTableNameWithoutAliases(selectQuery.From[0])
					if err != nil {
						return nil, err
					}
					out = append(out, &columnInfo{Table: tableNameStr, Name: allColumnsName, Alias: allColumnsName})
					continue
				}
				tableNameStr, err := findTableName(starExpr.TableName.Name.RawValue(), starExpr.TableName.Name.RawValue(), selectQuery.From)
				if err != nil {
					return nil, err
				}
				out = append(out, &columnInfo{Table: tableNameStr.Table, Name: allColumnsName, Alias: allColumnsName})
			}
			continue
		}
		out = append(out, nil)
	}
	return out, nil
}
