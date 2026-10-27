package configure

import (
	"errors"
	"fmt"

	"github.com/ClipMyHorseTV/livego/utils/uid"

	"github.com/go-redis/redis/v7"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
)

type RoomKeysType struct {
	redisCli   *redis.Client
	localCache *cache.Cache
}

var RoomKeys = &RoomKeysType{
	localCache: cache.New(cache.NoExpiration, 0),
}

var saveInLocal = true

func Init() {
	saveInLocal = len(Config.GetString("redis_addr")) == 0
	if saveInLocal {
		return
	}

	RoomKeys.redisCli = redis.NewClient(&redis.Options{
		Addr:     Config.GetString("redis_addr"),
		Password: Config.GetString("redis_pwd"),
		DB:       0,
	})

	_, err := RoomKeys.redisCli.Ping().Result()
	if err != nil {
		log.Panic("Redis: ", err)
	}

	log.Info("Redis connected")
}

// SetKey sets or resets a random key for channel.
func (r *RoomKeysType) SetKey(channel string) (key string, err error) {
	if !saveInLocal {
		for {
			key = uid.RandStringRunes(48)
			if _, err = r.redisCli.Get(key).Result(); errors.Is(err, redis.Nil) {
				err = r.redisCli.Set(channel, key, 0).Err()
				if err != nil {
					return key, err
				}

				err = r.redisCli.Set(key, channel, 0).Err()
				return key, err
			} else if err != nil {
				return key, err
			}
		}
	}

	for {
		key = uid.RandStringRunes(48)
		if _, found := r.localCache.Get(key); !found {
			r.localCache.SetDefault(channel, key)
			r.localCache.SetDefault(key, channel)
			break
		}
	}
	return key, err
}

func (r *RoomKeysType) GetKey(channel string) (newKey string, err error) {
	if !saveInLocal {
		if newKey, err = r.redisCli.Get(channel).Result(); errors.Is(err, redis.Nil) {
			newKey, err = r.SetKey(channel)
			log.Debugf("[KEY] new channel [%s]: %s", channel, newKey)
			return newKey, err
		}

		return newKey, err
	}

	if key, found := r.localCache.Get(channel); found {
		if s, ok := key.(string); ok {
			return s, nil
		}
	}
	newKey, err = r.SetKey(channel)
	log.Debugf("[KEY] new channel [%s]: %s", channel, newKey)
	return newKey, err
}

func (r *RoomKeysType) GetChannel(key string) (channel string, err error) {
	if !saveInLocal {
		return r.redisCli.Get(key).Result()
	}

	if chann, found := r.localCache.Get(key); found {
		if s, ok := chann.(string); ok {
			return s, nil
		}
	}
	return "", fmt.Errorf("%s does not exists", key)
}

func (r *RoomKeysType) DeleteChannel(channel string) bool {
	if !saveInLocal {
		return r.redisCli.Del(channel).Err() != nil
	}

	key, ok := r.localCache.Get(channel)
	if !ok {
		return false
	}
	r.localCache.Delete(channel)
	if s, ok := key.(string); ok {
		r.localCache.Delete(s)
	}
	return true
}

func (r *RoomKeysType) DeleteKey(key string) bool {
	if !saveInLocal {
		return r.redisCli.Del(key).Err() != nil
	}

	channel, ok := r.localCache.Get(key)
	if !ok {
		return false
	}
	if s, ok := channel.(string); ok {
		r.localCache.Delete(s)
	}
	r.localCache.Delete(key)
	return true
}
