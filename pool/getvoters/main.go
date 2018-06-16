package getvoters

import (
	"errors"
	"database/sql"
	"encoding/base64"
	_ "github.com/lib/pq"
)

const DB_DRIVER = "postgres"

const TOTALS_QUERY = `SELECT COUNT(accountid), SUM(balance)
FROM accounts WHERE inflationdest = $1`

const VOTERS_QUERY = `SELECT
accounts.accountid, balance, dataname, datavalue
FROM accounts LEFT JOIN accountdata
ON accountdata.accountid = accounts.accountid
AND dataname LIKE $2 WHERE inflationdest = $1`

// A single voter with its relevant data
type Voter struct {
	Balance string
	Data map[string]string
}
// The total number of voters, sum of votes, and the list of all voters
type Data struct {
  NumVoters string
  NumVotes string
  Voters map[string]*Voter
}

type DBconn struct {
  // PostgreSQL connection string
  Conn string
  // Pool address (inflation_dest of the voters)
  Pool string
  // Pattern for data names we are interested in
  Pattern string

  // Database SQL connection (internal)
  db *sql.DB
}

func NewDBconn(conn, pool, pattern string) (*DBconn, error) {

  // Validate the pool address received
  if len(pool) != 56 || pool[0] != 'G' {
    return nil, errors.New("ERROR: Invalid address provided")
  }

  // Try opening a connection to the database
  db, err := sql.Open(DB_DRIVER, conn)
  if err != nil {
    return nil, errors.New("ERROR opening DB connection: " + err.Error())
  }

  return &DBconn{conn, pool, pattern, db}, nil
}

func (c *DBconn) Close() error {
  if err := c.db.Close(); err != nil {
    return errors.New("ERROR closing DB connection: " + err.Error())
  }
  return nil
}

func (c *DBconn) GetTotals() (*Data, error) {
  // Values for the total number of voters and sum of votes
	var voters, votes string

  // QueryRow executes a query that is expected to return at most one row
  err := c.db.QueryRow(TOTALS_QUERY, c.Pool).Scan(&voters, &votes)
  if err != nil {
    return nil, errors.New("ERROR getting the sum of votes: " + err.Error())
  }

  // Returns a pointer to the struct (the map of Voters is nil)
  return &Data{NumVoters: voters, NumVotes: votes}, nil
}

func (c *DBconn) GetVoters() (*Data, error) {
  // Get the totals first (data is a pointer)
  data, err := c.GetTotals()
  if err != nil {
    return nil, err
  }

  // Make sure the Voters map is not nil (initialize an empty map)
  data.Voters = make(map[string]*Voter)

  // Only execute the query if we have voters
  if data.NumVoters != "0" {
    // Try getting the voters
    rows, err := c.db.Query(VOTERS_QUERY, c.Pool, c.Pattern)
    if err != nil {
      return nil, errors.New("ERROR getting the voters: " + err.Error())
    }
    defer rows.Close()

    // Loop all the rows (IDs and balances can repeat, name and value can be null)
  	for rows.Next() {
      var id, balance string
      var nameNull, valueNull sql.NullString

      // Copies the columns in the current row into the parameters
      err = rows.Scan(&id, &balance, &nameNull, &valueNull)
      if err != nil {
        return nil, errors.New("ERROR scanning query result row: " + err.Error())
      }

      // Get the voter for this ID in the map
      v := data.Voters[id]
      // Create a new one if it doesn't exist
  		if v == nil {
  			v = new(Voter)
        data.Voters[id] = v
  		}

      // Add this voter's balance to the map (update if repeated)
  		v.Balance = balance

      // Add the (key, value) pair, if it exists
  		if nameNull.Valid && valueNull.Valid {
        // We expect a base64 encoded string
  			decoded, err := base64.StdEncoding.DecodeString(valueNull.String)
  			// Ignore the data if we can't decode it
  			if err == nil {
  				// Adding data to a uninitialized map is a runtime panic
  				if v.Data == nil {
  					v.Data = make(map[string]string)
  				}
  				// Finally, add the data pair to the voter
  				v.Data[nameNull.String] = string(decoded)
  			}
  		}
    }
  }

  // Return the pointer, now with a valid map of Voters
  return data, nil
}
