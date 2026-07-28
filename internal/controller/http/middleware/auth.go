package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/pkg/auth"
)

// JWTAuth creates a middleware that validates JWT tokens.
func JWTAuth(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}
		if tokenString == "" {
			tokenString = c.GetHeader("User-Token")
		}
		if tokenString == "" {
			tokenString = c.GetHeader("token")
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": gin.H{"code": 202011502, "msg": "missing authorization header"},
			})
			c.Abort()
			return
		}

		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": gin.H{"code": 202011502, "msg": err.Error()},
			})
			c.Abort()
			return
		}

		// Only allow access tokens for API requests
		if claims.TokenType != auth.AccessToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": gin.H{"code": 202011502, "msg": "invalid token type"},
			})
			c.Abort()
			return
		}

		// Add claims to context
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)

		// Also set in Gin context for easy access
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("owner_type", claims.OwnerType)
		c.Set("owner_id", claims.OwnerID)

		c.Next()
	}
}

// OptionalAuth creates a middleware that validates JWT tokens but doesn't require them.
func OptionalAuth(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}

		claims, err := jwtManager.ValidateToken(parts[1])
		if err == nil && claims.TokenType == auth.AccessToken {
			ctx := auth.ContextWithClaims(c.Request.Context(), claims)
			c.Request = c.Request.WithContext(ctx)
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
		}

		c.Next()
	}
}
