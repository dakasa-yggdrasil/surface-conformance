package surface

import (
	// §3.1 — surface owning a SQL connection.
	_ "database/sql"

	// §3.3 — direct cloud SDK in a surface package.
	_ "github.com/aws/aws-sdk-go/aws"
)

// Sentinel — fixture only.
const Marker = "bad-surface-store"
