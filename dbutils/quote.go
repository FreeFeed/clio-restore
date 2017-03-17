package dbutils

// QuoteString quotes string value according to PostgreSQL rules
// see https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-STRINGS
func QuoteString(s string) string {
	const Q = 0x27 // single quote
	var (
		in  = []byte(s)
		out []byte
	)
	out = append(out, Q)
	for _, b := range in {
		if b == Q {
			out = append(out, Q)
		}
		out = append(out, b)
	}
	out = append(out, Q)

	return string(out)
}

// QuoteStrings quotes all strings in array
func QuoteStrings(ss []string) (out []string) {
	for _, s := range ss {
		out = append(out, QuoteString(s))
	}
	return
}
