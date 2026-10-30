package fixture

// Server serves.
type Server struct{}

// Listen binds addr.
func (s *Server) Listen(addr string) error { return nil }

// Run starts a server.
func Run() error {
	var s Server
	return s.Listen("localhost")
}
