package workspace

// nonWebPorts are the default ports of common services that browsers cannot
// talk to, so a link to them would mislead.
var nonWebPorts = map[int]bool{
	22:    true, // ssh
	1433:  true, // SQL Server
	1521:  true, // Oracle
	2181:  true, // ZooKeeper
	3306:  true, // MySQL
	5432:  true, // PostgreSQL
	5672:  true, // AMQP
	6379:  true, // Redis
	9092:  true, // Kafka
	11211: true, // memcached
	27017: true, // MongoDB
}

// webScheme guesses the URL scheme of the service on a target port from the
// port alone: https on 443 and 8443, none on a known non-web port, else http.
func webScheme(port int) (scheme string, isWeb bool) {
	switch {
	case port == 443 || port == 8443:
		return "https", true
	case nonWebPorts[port]:
		return "", false
	}
	return "http", true
}
