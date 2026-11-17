package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// CrossOrigin rejects state-changing requests that a browser sent from
// another site (Sec-Fetch-Site / Origin check from net/http). Requests
// without those headers come from non-browser clients, which carry no
// ambient cookie authority, and pass.
func CrossOrigin() gin.HandlerFunc {
	protection := http.NewCrossOriginProtection()
	return func(c *gin.Context) {
		if err := protection.Check(c.Request); err != nil {
			Fail(c, shared.ErrForbidden)
			return
		}
		c.Next()
	}
}
