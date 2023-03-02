package resources

type MonitoringResourceType string

const (
	MonitoringResourceTypeRedisInstance    MonitoringResourceType = "redis_instance"
	MonitoringResourceTypeCloudsqlDatabase MonitoringResourceType = "cloudsql_database"

	PostgresFreeStorageAverage    = "cro_postgres_free_storage_average"
	PostgresCPUUtilizationAverage = "cro_postgres_cpu_utilization_average"
	PostgresFreeableMemoryAverage = "cro_postgres_freeable_memory_average"

	RedisMemoryUsagePercentageAverage = "cro_redis_memory_usage_percentage_average"
	RedisFreeableMemoryAverage        = "cro_redis_freeable_memory_average"
	RedisCPUUtilizationAverage        = "cro_redis_cpu_utilization_average"
	RedisEngineCPUUtilizationAverage  = "cro_redis_engine_cpu_utilization_average"
)

func IsCompoundMetric(metric string) bool {
	for _, compoundMetric := range getCompoundMetrics() {
		if metric == compoundMetric {
			return true
		}
	}
	return false
}

func IsComputedCpuMetric(metric string) bool {
	for _, computedCpuMetric := range getComputedCpuMetrics() {
		if metric == computedCpuMetric {
			return true
		}
	}
	return false
}

func getCompoundMetrics() []string {
	return []string{
		RedisFreeableMemoryAverage,
		PostgresFreeStorageAverage,
		PostgresFreeableMemoryAverage,
	}
}

func getComputedCpuMetrics() []string {
	return []string{
		RedisCPUUtilizationAverage,
		RedisEngineCPUUtilizationAverage,
	}
}
