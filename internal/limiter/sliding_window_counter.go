package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var slidingWindowCounterScript = redis.NewScript(`
	local currentKey = KEYS[1]
	local prevKey = KEYS[2]
	local window = tonumber(ARGV[1])
	local now = tonumber(ARGV[2])
	local limit = tonumber(ARGV[3])

	local prevCmd = redis.call("GET", prevKey)
	local prevCount = 0
	if prevCmd ~= false then
		prevCount = tonumber(prevCmd)
	end

	local currentCount = tonumber(redis.call("GET", currentKey)) or 0

	local elapsed = (now % window) / window
	local overlap = 1 - elapsed

	local estimated = prevCount * overlap + currentCount

	if estimated >= limit then
		return {0, tostring(estimated)}
	end

	redis.call("INCR", currentKey)
	redis.call("EXPIRE", currentKey, 2 * window)

	return {1, tostring(estimated + 1)}
`)

func slidingWindowCounterAllow(ctx context.Context, client *redis.Client, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	now := time.Now()
	windowSeconds := int64(window.Seconds())

	currentWindow := now.Unix() / windowSeconds
	previousWindow := currentWindow - 1

	currentKey := fmt.Sprintf("rate:%s:%d", key, currentWindow)
	previousKey := fmt.Sprintf("rate:%s:%d", key, previousWindow)

	result, err := slidingWindowCounterScript.Run(
		ctx, client,
		[]string{currentKey, previousKey},
		windowSeconds, now.Unix(), limit,
	).Result()

	if err != nil {
		return false, 0, err
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1

	if !allowed {
		elapsedInWindow := now.Unix() % windowSeconds
		retryAfter := time.Duration(windowSeconds-elapsedInWindow) * time.Second
		return false, retryAfter, nil
	}

	return true, 0, nil
}