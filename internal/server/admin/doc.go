// Package admin provides the Admin HTTP server for managing the Straw Proxy system.
//
//	@title						Straw Proxy Admin Server API
//	@version					1.0
//	@description				Administrative API for managing the Straw Proxy system.
//	@description				Provides endpoints for API key management, routing rules, endpoints, fingerprints, usage, and cache.
//
//	@contact.name				Straw Proxy Support
//	@contact.url				https://github.com/kwilabs/straw-proxy-server
//
//	@license.name				MIT
//	@license.url				https://opensource.org/licenses/MIT
//
//	@host						localhost:8081
//	@BasePath					/admin
//
//	@securityDefinitions.apikey	AdminKeyAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token authentication (format: "Bearer <token>")
package admin
