package chessserver

import "testing"

func TestCreateAndJoin(t *testing.T) {
	tableName := "test table"
	ts := NewTableServer()
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ts.SendTableAction(CreateTableAction(tableName, p1))
	if !p1.GetTableResponse().Success {
		t.Error("expected to create table successfully")
	}
	if _, ok := ts.tables[tableName]; !ok {
		t.Error("expected table to be created")
	}
	if ts.tables[tableName].adminName != p1.playerModel.Name {
		t.Error("expected creator to be admin")
	}
	ts.SendTableAction(JoinTableAction(tableName, p1))
	if !p1.GetTableResponse().Success {
		t.Error("expected to join table successfully")
	}
	if p1.table != ts.tables[tableName] {
		t.Error("p1's table is not set after joining")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if !p2.GetTableResponse().Success {
		t.Error("expected to join table successfully")
	}
	if p2.table != ts.tables[tableName] {
		t.Error("p2's table is not set after joining")
	}
	if len(ts.tables[tableName].players) != 2 {
		t.Errorf("len(ts.tables[tableName].players) == %d, expected 2", len(ts.tables[tableName].players))
	}
}

func TestJoinFakeTable(t *testing.T) {
	tableName := "test table"
	ts := NewTableServer()
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ts.SendTableAction(CreateTableAction(tableName, p1))
	if !p1.GetTableResponse().Success {
		t.Error("expected to create table successfully")
	}
	ts.SendTableAction(JoinTableAction("fake table", p2))
	if p2.GetTableResponse().Success {
		t.Error("expected to fail to join table")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if !p2.GetTableResponse().Success {
		t.Error("expected to join table successfully")
	}
}
