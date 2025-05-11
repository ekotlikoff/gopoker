package chessserver

import (
	"fmt"
	"testing"

	model "github.com/ekotlikoff/gopoker/internal/model/table"
)

func TestCreateAndJoin(t *testing.T) {
	tableName := "test table"
	ts := NewTableServer()
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ts.SendTableAction(CreateTableAction(tableName, p1))
	if p1.GetTableResponse().Err != nil {
		t.Error("expected to create table successfully")
	}
	if _, ok := ts.tables[tableName]; !ok {
		t.Error("expected table to be created")
	}
	if ts.tables[tableName].adminName != p1.playerModel.Name {
		t.Error("expected creator to be admin")
	}
	ts.SendTableAction(JoinTableAction(tableName, p1))
	if p1.GetTableResponse().Err != nil {
		t.Error("expected to join table successfully")
	}
	if p1.table != ts.tables[tableName] {
		t.Error("p1's table is not set after joining")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if p2.GetTableResponse().Err != nil {
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
	if p1.GetTableResponse().Err != nil {
		t.Error("expected to create table successfully")
	}
	ts.SendTableAction(JoinTableAction("fake table", p2))
	if p2.GetTableResponse().Err == nil {
		t.Error("expected to fail to join table")
	}
	ts.SendTableAction(JoinTableAction(tableName, p2))
	if p2.GetTableResponse().Err != nil {
		t.Error("expected to join table successfully")
	}
}

func TestSit(t *testing.T) {
	tableName := "test table"
	ts := NewTableServer()
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	p1.playerModel.Funds = 1000
	p2.playerModel.Funds = 1000
	ts.SendTableAction(CreateTableAction(tableName, p1))
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p2))
	p2.GetTableResponse()
	p1Seat := 0
	ts.SendTableAction(SitTableAction(tableName, p1, p1Seat))
	if err := p1.GetTableResponse().Err; err != nil {
		t.Errorf("expected to sit at table successfully, failed with error: %s", err)
	}
	seat := 99
	ts.SendTableAction(SitTableAction(tableName, p2, seat))
	if p2.GetTableResponse().Err == nil {
		t.Errorf("expected not to sit at seat %d successfully", seat)
	}
	ts.SendTableAction(SitTableAction(tableName, p2, p1Seat))
	if p2.GetTableResponse().Err == nil {
		t.Errorf("expected not to sit at seat %d successfully", seat)
	}
	ts.SendTableAction(SitTableAction(tableName, p2, 3))
	if err := p2.GetTableResponse().Err; err != nil {
		t.Errorf("expected to sit successfully, failed with error: %s", err)
	}
}

func consumeTableUpdates(ps ...*Player) {
	for _, p := range ps {
		<-p.TableUpdateChan
	}
}

func createTableWithTwoPlayers(tableName string) (*TableServer, *Player, *Player) {
	ts := NewTableServer()
	go ts.Serve()
	p1 := NewPlayer("1")
	p2 := NewPlayer("2")
	ps := []*Player{p1, p2}
	p1.playerModel.Funds = 1000
	p2.playerModel.Funds = 1000
	ts.SendTableAction(CreateTableAction(tableName, p1))
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p1))
	consumeTableUpdates(p1)
	p1.GetTableResponse()
	ts.SendTableAction(JoinTableAction(tableName, p2))
	consumeTableUpdates(ps...)
	p2.GetTableResponse()
	ts.SendTableAction(SitTableAction(tableName, p1, 1))
	consumeTableUpdates(ps...)
	p1.GetTableResponse()
	ts.SendTableAction(SitTableAction(tableName, p2, 3))
	consumeTableUpdates(ps...)
	p2.GetTableResponse()
	return ts, p1, p2
}

func TestSimpleHand(t *testing.T) {
	tableName := "test table"
	ts, p1, p2 := createTableWithTwoPlayers(tableName)
	ps := []*Player{p1, p2}
	ts.SendTableAction(StartTableAction(tableName, p2))
	if r := p2.GetTableResponse(); r.Err == nil {
		t.Error("only the admin should be able to start the table")
	}
	ts.SendTableAction(StartTableAction(tableName, p1))
	if r := p1.GetTableResponse(); r.Err != nil {
		t.Error("the admin should be able to start the table")
	}
	consumeTableUpdates(ps...)
	table := ts.tables[tableName]
	fmt.Println(table.table)
	p2.SendRoundAction(model.RoundAction{ActionType: model.Call})
	if r := p2.GetRoundResponse(); r.Err != nil {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	consumeTableUpdates(ps...)
	fmt.Println(table.table)
	p1.SendRoundAction(model.RoundAction{ActionType: model.Call})
	if r := p1.GetRoundResponse(); r.Err != nil {
		t.Errorf("expected a successful bet, got error: %s", r.Err)
	}
	fmt.Println(table.table)
	// consumeTableUpdates(ps...)
	// p1.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p1.GetRoundResponse()
	// consumeTableUpdates(ps...)
	//
	//	if !table.playing {
	//		t.Error("table should be playing")
	//	}
	//
	// time.Sleep(time.Second)
	//
	//	if len(table.table.Board()) != 3 {
	//		t.Errorf("expected the flop, len(table.table.Board())==%d", len(table.table.Board()))
	//	}
	//
	// ts.SendTableAction(StandTableAction(tableName, p1))
	// p1.GetTableResponse()
	// consumeTableUpdates(ps...)
	// ts.SendTableAction(StandTableAction(tableName, p2))
	// p2.GetTableResponse()
	// consumeTableUpdates(ps...)
	// // TODO add timeouts for the server's SendRoundResponse
	// p1.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p1.GetRoundResponse()
	// consumeTableUpdates(ps...)
	// p2.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p2.GetRoundResponse()
	// consumeTableUpdates(ps...)
	// p1.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p1.GetRoundResponse()
	// consumeTableUpdates(ps...)
	// p2.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p2.GetRoundResponse()
	// consumeTableUpdates(ps...)
	// p1.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p1.GetRoundResponse()
	// consumeTableUpdates(ps...)
	// p2.SendRoundAction(model.RoundAction{ActionType: model.Call})
	// p2.GetRoundResponse()
	// consumeTableUpdates(ps...)
	//
	//	go func() {
	//		update := <-p1.TableUpdateChan
	//		if len(update.StateUpdate.NowStanding) != 2 {
	//			t.Errorf("expected two standers, got %d", len(update.StateUpdate.NowStanding))
	//		}
	//		update = <-p1.TableUpdateChan
	//		if !update.StateUpdate.PlayStopped {
	//			t.Error("expected play to stop after standing")
	//		}
	//	}()
	//
	//	go func() {
	//		update := <-p2.TableUpdateChan
	//		if len(update.StateUpdate.NowStanding) != 2 {
	//			t.Errorf("expected two standers, got %d", len(update.StateUpdate.NowStanding))
	//		}
	//		update = <-p2.TableUpdateChan
	//		if !update.StateUpdate.PlayStopped {
	//			t.Error("expected play to stop after standing")
	//		}
	//	}()
}

// TODO test pause
