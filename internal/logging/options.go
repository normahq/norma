package logging

//go:generate go tool options-gen -from-struct=Options -out-filename=options_generated.go
type Options struct {
	debug bool `option:"default:false" mapstructure:"debug"`
	trace bool `option:"default:false" mapstructure:"trace"`
	json  bool `option:"default:false" mapstructure:"json"`
}
