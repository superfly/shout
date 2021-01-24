package shout

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/hashicorp/serf/serf"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type Shout struct {
	db     *sql.DB
	logger *logrus.Logger
	cfg    *Config
}

type Config struct {
	NodeName         string
	DBPath           string
	RPCAddr          string
	MemberlistPort   int
	HandlersPath     string
	MigrationsSource string

	Logger *logrus.Logger
}

func New(cfg *Config) (*Shout, error) {
	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		f, err := os.Create(cfg.DBPath)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Shout{db: db, logger: cfg.Logger, cfg: cfg}, nil
}

func (shout *Shout) Run() {
	log := shout.logger
	log.Printf("%+v\n", shout)

	if err := shout.migrate(); err != nil {
		log.Fatalf("error running migrations: %v", err)
	}

	serfCfg := serf.DefaultConfig()
	serfCfg.NodeName = shout.cfg.NodeName
	serfCfg.MemberlistConfig = memberlist.DefaultWANConfig()
	serfCfg.MemberlistConfig.Name = shout.cfg.NodeName
	serfCfg.MemberlistConfig.BindPort = shout.cfg.MemberlistPort

	agentCfg := agent.DefaultConfig()
	agentCfg.RPCAddr = shout.cfg.RPCAddr
	log.Printf("agent cfg: %+v\n", agentCfg)
	a, err := agent.Create(agentCfg, serfCfg, log.Writer())
	if err != nil {
		log.Fatalf("could not create agent: %v", err)
	}

	ch := make(chan serf.Event)
	a.SerfConfig().EventCh = ch

	log.Println("staring serf agent")
	if err := a.Start(); err != nil {
		log.Fatalf("error starting serf agent: %v", err)
	}

	rpcLn, err := net.Listen("tcp", shout.cfg.RPCAddr)
	if err != nil {
		log.Fatalf("error listening for serf agent: %v", err)
	}

	agent.NewAgentIPC(a, "", rpcLn, log.Writer(), nil)

	shutdownCh := a.ShutdownCh()

OUTIE:
	for {
		select {
		case evt := <-ch:
			log.Println("received event: ", evt)
			switch evt.EventType() {
			case serf.EventQuery:
				v := evt.(*serf.Query)
				p := fmt.Sprintf("%s/%s.sql", path.Join(shout.cfg.HandlersPath, "queries"), v.Name)
				buf, err := ioutil.ReadFile(p)
				if err != nil {
					log.Errorf("error reading event file %s: %v", p, err)
					continue
				}
				var args map[string]interface{}
				if err := json.Unmarshal(v.Payload, &args); err != nil {
					log.Errorf("error parsing json for event %s, json: %s => %v", p, string(v.Payload), err)
					continue
				}

				func() {
					ctx := context.Background()
					conn, err := shout.db.Conn(ctx)
					if err != nil {
						log.Errorf("error getting db conn: %v", err)
						return
					}
					defer conn.Close()
					sqlArgs := make([]interface{}, len(args))
					for k, v := range args {
						sqlArgs = append(sqlArgs, sql.Named(k, v))
					}
					rows, err := conn.QueryContext(ctx, string(buf), sqlArgs...)
					if err != nil {
						log.Errorf("error executing sql query %s => %v", string(buf), err)
						return
					}
					log.Printf("rows: %+v\n", rows)

					j, err := rowsToJSON(rows)
					if err != nil {
						log.Errorf("error converting rows to json %+v => %v", rows, err)
						return
					}
					if err := v.Respond(j); err != nil {
						log.Errorf("error responding to query: %v", err)
					}
				}()

			case serf.EventUser:
				log.Println("user event!")
				v := evt.(serf.UserEvent)
				p := fmt.Sprintf("%s/%s.sql", path.Join(shout.cfg.HandlersPath, "events"), v.Name)
				buf, err := ioutil.ReadFile(p)
				if err != nil {
					log.Errorf("error reading event file %s: %v", p, err)
					continue
				}
				var args map[string]interface{}
				if err := json.Unmarshal(v.Payload, &args); err != nil {
					log.Errorf("error parsing json for event %s, json: %s => %v", p, string(v.Payload), err)
					continue
				}

				func() {
					ctx := context.Background()
					conn, err := shout.db.Conn(ctx)
					if err != nil {
						log.Errorf("error getting db conn: %v", err)
						return
					}
					defer conn.Close()
					sqlArgs := make([]interface{}, len(args))
					for k, v := range args {
						sqlArgs = append(sqlArgs, sql.Named(k, v))
					}
					res, err := conn.ExecContext(ctx, string(buf), sqlArgs...)
					if err != nil {
						log.Errorf("error executing sql query %s => %v", string(buf), err)
						return
					}
					log.Printf("res: %+v\n", res)
				}()
			}
		case <-shutdownCh:
			log.Println("serf shat down")
			break OUTIE
		}
	}
	log.Println("shutting down")
}

func (s *Shout) migrate() error {
	dbDriver, err := sqlite3.WithInstance(s.db, &sqlite3.Config{MigrationsTable: sqlite3.DefaultMigrationsTable, DatabaseName: "shout"})
	if err != nil {
		return fmt.Errorf("could not prepare db for migration: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(s.cfg.MigrationsSource, "sqlite3", dbDriver)
	if err != nil {
		return fmt.Errorf("could not prepare migration instance: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("could not apply db migrations: %v", err)
	}
	return nil
}

func rowsToJSON(r *sql.Rows) ([]byte, error) {
	rows := []interface{}{}

	columns, err := r.Columns()
	if err != nil {
		return nil, err
	}

	for r.Next() {

		values := make([]interface{}, len(columns))
		for i := range values {
			values[i] = new(interface{})
		}

		err = r.Scan(values...)
		if err != nil {
			return nil, err
		}

		dest := map[string]interface{}{}

		for i, column := range columns {
			dest[column] = *(values[i].(*interface{}))
		}

		rows = append(rows, dest)
	}

	return json.Marshal(rows)
}
