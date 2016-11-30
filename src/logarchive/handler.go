package logarchive

type RedisHandler struct {
	Shield map[string]bool
	Config map[string]interface{}
}

func (this *RedisHandler) SetShield(name string) (*RedisHandler) {
	if this.Shield == nil {
		this.Shield = make(map[string]bool)
	}

	this.Shield[name] = true
	return this
}

func (this *RedisHandler) SetConfig(config map[string]interface{}) (*RedisHandler) {
	if this.Config == nil {
		this.Config = make(map[string]interface{})
	}

	this.Config = config
	return this
}