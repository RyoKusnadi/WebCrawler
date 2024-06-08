package db

import (
	"database/sql"
	"os"
	"testing"
	"webcrawler/linkgraph/graph/graphtest"

	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(new(DbGraphTestSuite))

func Test(t *testing.T) { gc.TestingT(t) }

type DbGraphTestSuite struct {
	graphtest.SuiteBase
	db *sql.DB
}

func (s *DbGraphTestSuite) SetUpSuite(c *gc.C) {
	dsn := os.Getenv("CDB_DSN")
	if dsn == "" {
		c.Skip("Missing CDB_DSN envvar; skipping db graph test suite")
	}

	g, err := NewDBGraph(dsn)
	c.Assert(err, gc.IsNil)
	s.SetGraph(g)
	s.db = g.db
}

func (s *DbGraphTestSuite) SetUpTest(c *gc.C) {
	s.flushDB(c)
}

func (s *DbGraphTestSuite) TearDownSuite(c *gc.C) {
	if s.db != nil {
		s.flushDB(c)
		c.Assert(s.db.Close(), gc.IsNil)
	}
}

func (s *DbGraphTestSuite) flushDB(c *gc.C) {
	_, err := s.db.Exec("DELETE FROM links")
	c.Assert(err, gc.IsNil)
	_, err = s.db.Exec("DELETE FROM edges")
	c.Assert(err, gc.IsNil)
}
