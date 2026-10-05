package exchange

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

type Engine struct {
	mu               sync.RWMutex
	db               *sql.DB
	clock            Clock
	state            State
	players          map[int64]*Player
	names            map[string]int64
	uploads          map[string]int64
	watchers         map[int64]map[int64]bool
	marginOwners     map[int64]bool
	activePlayers    map[int64]bool
	delistCandidates map[int64]bool
	instances        map[string]int64
	instanceIDs      map[string]int64
	identityCodes    map[string]int64
	RequireBinding   bool
	dirty            map[int64]bool
	before           map[int64]*Player
	histories        []historyWrite
	trades           []Trade
	stockVersions    map[int64]uint64
	detailCache      map[string]detailCacheEntry
	detailBytes      int
	points           []struct {
		ID    int64
		Point QuotePoint
	}
	archives      []SeasonResult
	cache         []byte
	cacheRevision uint64
	cacheTime     int64
	cacheUntil    int64
	cacheVersion  uint64
	changes       chan struct{}
}

func Open(path string, clock Clock) (*Engine, error) {
	if clock == nil {
		clock = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000; PRAGMA cache_size=-2048; PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS metadata (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS players (id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, folded TEXT NOT NULL UNIQUE, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS player_identity_codes (player_id INTEGER PRIMARY KEY REFERENCES players(id), code TEXT NOT NULL UNIQUE);
CREATE TABLE IF NOT EXISTS quotes (stock_id INTEGER NOT NULL, time INTEGER NOT NULL, price INTEGER NOT NULL, PRIMARY KEY(stock_id,time));
CREATE TABLE IF NOT EXISTS quote_history (stock_id INTEGER NOT NULL, time INTEGER NOT NULL, price INTEGER NOT NULL, precise INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(stock_id,time));
CREATE TABLE IF NOT EXISTS minute_prices (stock_id INTEGER NOT NULL, time INTEGER NOT NULL, open INTEGER NOT NULL, high INTEGER NOT NULL, low INTEGER NOT NULL, close INTEGER NOT NULL, first_time INTEGER NOT NULL, last_time INTEGER NOT NULL, samples INTEGER NOT NULL, PRIMARY KEY(stock_id,time));
CREATE INDEX IF NOT EXISTS minute_prices_time_stock ON minute_prices(time,stock_id);
CREATE TABLE IF NOT EXISTS trades (id INTEGER PRIMARY KEY, stock_id INTEGER NOT NULL, username TEXT NOT NULL, side TEXT NOT NULL, kind TEXT NOT NULL, quantity INTEGER NOT NULL, price INTEGER NOT NULL, time INTEGER NOT NULL, forced INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS trades_stock_time ON trades(stock_id,time,id);
CREATE TABLE IF NOT EXISTS history_sync (stock_id INTEGER NOT NULL, month TEXT NOT NULL, count INTEGER NOT NULL, digest TEXT NOT NULL, checked_at INTEGER NOT NULL, PRIMARY KEY(stock_id,month));
CREATE TABLE IF NOT EXISTS receipts (player_id INTEGER NOT NULL, client_id TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(player_id,client_id));
CREATE TABLE IF NOT EXISTS instance_bindings (binding_key TEXT PRIMARY KEY, source_key TEXT UNIQUE NOT NULL, player_id INTEGER UNIQUE NOT NULL);
CREATE TABLE IF NOT EXISTS seasons (id TEXT PRIMARY KEY, data TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	e := &Engine{db: db, clock: clock, players: map[int64]*Player{}, names: map[string]int64{}, uploads: map[string]int64{}, watchers: map[int64]map[int64]bool{}, changes: make(chan struct{})}
	var raw string
	err = db.QueryRow("SELECT data FROM metadata WHERE id=1").Scan(&raw)
	if err == sql.ErrNoRows {
		e.state = State{Settings: DefaultSettings(), NextPlayer: 1, NextOrder: 1}
		e.state.Season = seasonFor(clock(), e.state.Settings)
	} else if err != nil {
		db.Close()
		return nil, err
	} else if err = json.Unmarshal([]byte(raw), &e.state); err != nil {
		db.Close()
		return nil, err
	}
	if raw != "" {
		migrateFinancingRules(raw, &e.state.Settings)
		migrateDelistThreshold(raw, &e.state.Settings)
	}
	if err = ValidateSettings(e.state.Settings); err != nil {
		db.Close()
		return nil, err
	}
	rows, err := db.Query("SELECT data FROM players")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rows.Next() {
		var v string
		var p storedPlayer
		if err = rows.Scan(&v); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(v), &p); err != nil {
			break
		}
		p.Player.Folded = p.Folded
		p.Player.PasswordHash = p.PasswordHash
		p.Player.UploadHash = p.UploadHash
		p.Player.SessionVersion = p.SessionVersion
		if p.Player.InitialCash == 0 {
			p.Player.InitialCash = InitialCash
		}
		e.players[p.Player.ID] = p.Player
		e.names[p.Folded] = p.Player.ID
		e.uploads[p.UploadHash] = p.Player.ID
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		db.Close()
		return nil, fmt.Errorf("读取玩家: %v %v", err, rowErr)
	}
	e.reindex()
	e.stockVersions = map[int64]uint64{}
	e.detailCache = map[string]detailCacheEntry{}
	if err = e.migrateIdentityCodes(); err != nil {
		db.Close()
		return nil, err
	}
	if err = e.migrateMarketData(); err != nil {
		db.Close()
		return nil, err
	}
	if err = e.Tick(); err != nil {
		db.Close()
		return nil, err
	}
	return e, nil
}
func (e *Engine) Close() error { return e.db.Close() }
func clonePlayer(p *Player) *Player {
	b, _ := json.Marshal(p)
	var c Player
	_ = json.Unmarshal(b, &c)
	c.Folded = p.Folded
	c.PasswordHash = p.PasswordHash
	c.UploadHash = p.UploadHash
	c.SessionVersion = p.SessionVersion
	return &c
}
func (e *Engine) touch(id int64) *Player {
	if !e.dirty[id] {
		e.before[id] = e.players[id]
		e.players[id] = clonePlayer(e.players[id])
		e.dirty[id] = true
	}
	return e.players[id]
}
func (e *Engine) reindex() {
	e.instances = map[string]int64{}
	e.instanceIDs = map[string]int64{}
	e.identityCodes = map[string]int64{}
	e.watchers = map[int64]map[int64]bool{}
	e.marginOwners = map[int64]bool{}
	e.activePlayers = map[int64]bool{}
	e.delistCandidates = map[int64]bool{}
	for id, p := range e.players {
		if p.IdentityCode != "" {
			e.identityCodes[p.IdentityCode] = id
		}
		if needsTick(p) {
			e.activePlayers[id] = true
		}
		if e.belowDelistThreshold(p) {
			e.delistCandidates[id] = true
		}
		if p.Binding != nil {
			e.instances[p.Binding.Key] = id
			e.instanceIDs[p.Binding.InstanceID] = id
		}
		if hasMargin(p) {
			e.marginOwners[id] = true
		}
		for stock := range p.Positions {
			if e.watchers[stock] == nil {
				e.watchers[stock] = map[int64]bool{}
			}
			e.watchers[stock][id] = true
		}
		for _, o := range p.Orders {
			if o.Status == "pending" {
				if e.watchers[o.StockID] == nil {
					e.watchers[o.StockID] = map[int64]bool{}
				}
				e.watchers[o.StockID][id] = true
			}
		}
	}
}

// 单写事务中只复制和落盘受影响的账户；提交失败时恢复内存，避免双重成交。
func (e *Engine) transaction(fn func() error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	before := map[int64]*Player{}
	e.before = before
	stateBefore := e.state
	e.dirty = map[int64]bool{}
	e.points = nil
	e.archives = nil
	e.histories = nil
	e.trades = nil
	defer func() {
		e.before = nil
		e.dirty = nil
		e.points = nil
		e.archives = nil
		e.histories = nil
		e.trades = nil
	}()
	rollback := func() {
		for id := range e.dirty {
			if p := before[id]; p != nil {
				e.players[id] = p
			} else {
				delete(e.players, id)
			}
		}
		e.state = stateBefore
	}
	if err := fn(); err != nil {
		rollback()
		return err
	}
	if len(e.dirty) == 0 && len(e.histories) == 0 && reflect.DeepEqual(e.state, stateBefore) {
		return nil
	}
	e.state.Revision++
	tx, err := e.db.Begin()
	if err != nil {
		rollback()
		return err
	}
	defer tx.Rollback()
	changedStocks := map[int64]bool{}
	for id := range e.dirty {
		p := e.players[id]
		if before[id] == nil || before[id].Quote != p.Quote || before[id].Disabled != p.Disabled || before[id].Delisted != p.Delisted || before[id].Username != p.Username {
			changedStocks[id] = true
		}
		if before[id] != nil && before[id].Username != p.Username {
			for _, o := range p.Orders {
				if o.Status == "pending" {
					changedStocks[o.StockID] = true
				}
			}
		}
		b, marshalErr := json.Marshal(storedPlayer{Player: p, Folded: p.Folded, PasswordHash: p.PasswordHash, UploadHash: p.UploadHash, SessionVersion: p.SessionVersion})
		if marshalErr != nil {
			err = marshalErr
			break
		}
		_, err = tx.Exec("INSERT INTO players(id,username,folded,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET username=excluded.username,folded=excluded.folded,data=excluded.data", id, p.Username, p.Folded, string(b))
		if err != nil {
			break
		}
		_, err = tx.Exec("INSERT INTO player_identity_codes(player_id,code) VALUES(?,?) ON CONFLICT(player_id) DO UPDATE SET code=excluded.code", id, p.IdentityCode)
		if err != nil {
			break
		}
		if p.Binding != nil {
			_, err = tx.Exec("INSERT INTO instance_bindings(binding_key,source_key,player_id) VALUES(?,?,?) ON CONFLICT(binding_key) DO NOTHING", p.Binding.Key, p.Binding.InstanceID, id)
			if err != nil {
				break
			}
		}
		oldOrders := map[int64]*Order{}
		if before[id] != nil {
			for _, o := range before[id].Orders {
				oldOrders[o.ID] = o
			}
		}
		for _, o := range p.Orders {
			old := oldOrders[o.ID]
			if old != nil && old.Status == o.Status && old.Reason == o.Reason {
				continue
			}
			if o.Status == "pending" || old != nil && old.Status == "pending" {
				changedStocks[o.StockID] = true
			}
			raw, _ := json.Marshal(o)
			_, err = tx.Exec("INSERT INTO receipts(player_id,client_id,data) VALUES(?,?,?) ON CONFLICT(player_id,client_id) DO UPDATE SET data=excluded.data", id, o.ClientID, string(raw))
			if err != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		b, _ := json.Marshal(e.state)
		_, err = tx.Exec("INSERT INTO metadata(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(b))
	}
	for _, v := range e.points {
		if err != nil {
			break
		}
		_, err = tx.Exec("INSERT INTO quotes(stock_id,time,price) VALUES(?,?,?) ON CONFLICT(stock_id,time) DO UPDATE SET price=excluded.price", v.ID, v.Point.Time, v.Point.Price)
	}
	if err == nil {
		err = e.writeMarketData(tx, changedStocks)
	}
	for _, v := range e.archives {
		if err != nil {
			break
		}
		b, _ := json.Marshal(v)
		_, err = tx.Exec("INSERT INTO seasons(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", v.Season.ID, string(b))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		rollback()
		return err
	}
	for stock := range changedStocks {
		e.stockVersions[stock]++
	}
	// 更新受影响账户的订阅索引，报价变化只触达持仓或挂单玩家。
	for id := range e.dirty {
		old := before[id]
		if old != nil {
			if old.Folded != e.players[id].Folded {
				delete(e.names, old.Folded)
			}
			for stock := range old.Positions {
				delete(e.watchers[stock], id)
			}
			for _, o := range old.Orders {
				delete(e.watchers[o.StockID], id)
			}
		}
		p := e.players[id]
		if p.Binding != nil {
			e.instances[p.Binding.Key] = id
			e.instanceIDs[p.Binding.InstanceID] = id
		}
		delete(e.marginOwners, id)
		delete(e.activePlayers, id)
		delete(e.delistCandidates, id)
		if needsTick(p) {
			e.activePlayers[id] = true
		}
		if e.belowDelistThreshold(p) {
			e.delistCandidates[id] = true
		}
		if hasMargin(p) {
			e.marginOwners[id] = true
		}
		for stock := range p.Positions {
			if e.watchers[stock] == nil {
				e.watchers[stock] = map[int64]bool{}
			}
			e.watchers[stock][id] = true
		}
		for _, o := range p.Orders {
			if o.Status == "pending" {
				if e.watchers[o.StockID] == nil {
					e.watchers[o.StockID] = map[int64]bool{}
				}
				e.watchers[o.StockID][id] = true
			}
		}
		e.names[p.Folded] = id
		e.identityCodes[p.IdentityCode] = id
		e.uploads[p.UploadHash] = id
	}
	e.cache = nil
	if e.changes != nil {
		close(e.changes)
	}
	e.changes = make(chan struct{})
	return nil
}

func needsTick(p *Player) bool {
	if hasMargin(p) || len(p.Settlements) > 0 {
		return true
	}
	for _, o := range p.Orders {
		if o.Status == "pending" {
			return true
		}
	}
	return false
}

func (e *Engine) History(id int64) ([]QuotePoint, error) {
	rows, err := e.db.Query("SELECT time/1000,price FROM quote_history WHERE stock_id=? ORDER BY time", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuotePoint{}
	for rows.Next() {
		var p QuotePoint
		if err := rows.Scan(&p.Time, &p.Price); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (e *Engine) Seasons() ([]SeasonResult, error) {
	rows, err := e.db.Query("SELECT data FROM seasons ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SeasonResult{}
	for rows.Next() {
		var raw string
		var v SeasonResult
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
