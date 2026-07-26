package limiter

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var tokenBucketScript = redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])  -- tokens per second
		local now = tonumber(ARGV[3])

		local bucket = redis.call("HMGET", key, "tokens", "last_refill")
		local tokens = tonumber(bucket[1])
		local last_refill = tonumber(bucket[2])

		if tokens == nil then
			tokens = capacity
			last_refill = now
		end

		-- refill based on elapsed time since last check
		local elapsed = math.max(0, now - last_refill)
		tokens = math.min(capacity, tokens + (elapsed * refill_rate))

		local allowed = 0
		if tokens >= 1 then
			tokens = tokens - 1
			allowed = 1
		end

		redis.call("HMSET", key, "tokens", tokens, "last_refill", now)
		redis.call("EXPIRE", key, 3600)

		return {allowed, tokens}
`)


func tokenBucketAllow(ctx context.Context,client *redis.Client,key string,limit int,window time.Duration) (bool , time.Duration, error) {
	refillRate := float64(limit) / window.Seconds()
	now := float64(time.Now().UnixMilli()) / 1000.0

	result, err := tokenBucketScript.Run(ctx, client, []string{key}, limit, refillRate, now).Result()

	if err != nil {
		return false, 0, err
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1

	if !allowed {
		retryAfter := time.Duration(1.0/refillRate*1000) * time.Millisecond
		return false, retryAfter, nil
	}
	return true, 0, nil

}

