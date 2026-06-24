package msg

type HealthConfig struct {
	HealthCheckType          string
	HealthCheckConfig        string
	HealthCheckSkipSSLVerify bool
	HealthCheckTimeout       uint32
	HealthCheckInterval      uint32
	HealthCheckMaxFailures   uint32
	HealthCheckAutoRestart   bool
}
