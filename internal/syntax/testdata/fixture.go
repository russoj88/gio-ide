package fixture

import "fmt"

// Hello greets name.
func Hello(name string) string {
	return fmt.Sprintf("hi %s", name)
}
