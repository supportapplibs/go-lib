package databases

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/goravel/framework/facades"
	"github.com/redis/go-redis/v9"
	"github.com/supportapplibs/go-lib/log"
)

var (
	RedisClient *redis.Client
	RedisOnce   sync.Once
)

func GetRedisClient() *redis.Client {
	RedisOnce.Do(func() {
		RedisClient = redis.NewClient(&redis.Options{
			Addr:     facades.Config().GetString("database.redis.default.host") + ":" + facades.Config().GetString("database.redis.default.port"),
			Password: facades.Config().GetString("database.redis.default.password"),
			DB:       facades.Config().GetInt("database.redis.default.database"),

			// Connection Pool Configuration
			PoolSize:     facades.Config().GetInt("database.redis.default.pool_size", 100),                               // Max number of socket connections (default: 10 * CPU cores)
			MinIdleConns: facades.Config().GetInt("database.redis.default.min_idle_conns", 10),                           // Min idle connections for quick response
			MaxIdleConns: facades.Config().GetInt("database.redis.default.max_idle_conns", 50),                           // Max idle connections
			PoolTimeout:  time.Duration(facades.Config().GetInt("database.redis.default.pool_timeout", 4)) * time.Second, // Wait time for connection

			// Connection Lifetime Management
			ConnMaxIdleTime: time.Duration(facades.Config().GetInt("database.redis.default.conn_max_idle_time", 300)) * time.Second, // 5 minutes - close idle connections
			ConnMaxLifetime: time.Duration(facades.Config().GetInt("database.redis.default.conn_max_lifetime", 3600)) * time.Second, // 1 hour - max connection lifetime

			// Timeout Configuration
			DialTimeout:  5 * time.Second, // Connection timeout
			ReadTimeout:  3 * time.Second, // Socket read timeout
			WriteTimeout: 3 * time.Second, // Socket write timeout

			// Pool FIFO - helps close idle connections faster
			PoolFIFO: true,
		})

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := RedisClient.Ping(ctx).Err(); err != nil {
			log.Runtime(ctx).Error(log.Map{
				"msg": fmt.Sprintf("failed to ping redis: %s", err),
			})
		} else {
			log.Runtime(ctx).Error(log.Map{
				"msg": "Redis connection established successfully",
			})
		}
	})
	return RedisClient
}

// GetPoolStats returns current pool statistics
func GetPoolStats() *redis.PoolStats {
	if RedisClient != nil {
		return RedisClient.PoolStats()
	}
	return nil
}

// CloseRedisClient gracefully closes the Redis connection
func CloseRedisClient() error {
	if RedisClient != nil {
		return RedisClient.Close()
	}
	return nil
}
