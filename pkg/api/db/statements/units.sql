INSERT INTO units (cluster_id,resource_manager,uuid,name,project,groupname,username,created_at,started_at,ended_at,created_at_ts,started_at_ts,ended_at_ts,elapsed,state,allocation,total_time_seconds,avg_cpu_usage,avg_cpu_mem_usage,total_cpu_energy_usage_kwh,total_cpu_emissions_gms,avg_gpu_usage,avg_gpu_mem_usage,total_gpu_energy_usage_kwh,total_gpu_emissions_gms,total_io_write_stats,total_io_read_stats,total_ingress_stats,total_outgress_stats,tags,ignore,num_updates,last_updated_at) VALUES (:cluster_id,:resource_manager,:uuid,:name,:project,:groupname,:username,:created_at,:started_at,:ended_at,:created_at_ts,:started_at_ts,:ended_at_ts,:elapsed,:state,:allocation,:total_time_seconds,:avg_cpu_usage,:avg_cpu_mem_usage,:total_cpu_energy_usage_kwh,:total_cpu_emissions_gms,:avg_gpu_usage,:avg_gpu_mem_usage,:total_gpu_energy_usage_kwh,:total_gpu_emissions_gms,:total_io_write_stats,:total_io_read_stats,:total_ingress_stats,:total_outgress_stats,:tags,:ignore,:num_updates,:last_updated_at) ON CONFLICT(cluster_id,uuid,started_at) DO UPDATE SET
  ended_at = :ended_at,
  ended_at_ts = :ended_at_ts,
  elapsed = :elapsed,
  state = :state,
  total_time_seconds = add_metric_map(total_time_seconds, :total_time_seconds),
  avg_cpu_usage = avg_metric_map(avg_cpu_usage, :avg_cpu_usage, CAST(json_extract(total_time_seconds, '$.alloc_cputime') AS REAL), CAST(json_extract(:total_time_seconds, '$.alloc_cputime') AS REAL)),
  avg_cpu_mem_usage = avg_metric_map(avg_cpu_mem_usage, :avg_cpu_mem_usage, CAST(json_extract(total_time_seconds, '$.alloc_cpumemtime') AS REAL), CAST(json_extract(:total_time_seconds, '$.alloc_cpumemtime') AS REAL)),
  total_cpu_energy_usage_kwh = add_metric_map(total_cpu_energy_usage_kwh, :total_cpu_energy_usage_kwh),
  total_cpu_emissions_gms = add_metric_map(total_cpu_emissions_gms, :total_cpu_emissions_gms),
  avg_gpu_usage = avg_metric_map(avg_gpu_usage, :avg_gpu_usage, CAST(json_extract(total_time_seconds, '$.alloc_gputime') AS REAL), CAST(json_extract(:total_time_seconds, '$.alloc_gputime') AS REAL)),
  avg_gpu_mem_usage = avg_metric_map(avg_gpu_mem_usage, :avg_gpu_mem_usage, CAST(json_extract(total_time_seconds, '$.alloc_gpumemtime') AS REAL), CAST(json_extract(:total_time_seconds, '$.alloc_gpumemtime') AS REAL)),
  total_gpu_energy_usage_kwh = add_metric_map(total_gpu_energy_usage_kwh, :total_gpu_energy_usage_kwh),
  total_gpu_emissions_gms = add_metric_map(total_gpu_emissions_gms, :total_gpu_emissions_gms),
  total_io_write_stats = add_metric_map(total_io_write_stats, :total_io_write_stats),
  total_io_read_stats = add_metric_map(total_io_read_stats, :total_io_read_stats),
  total_ingress_stats = add_metric_map(total_ingress_stats, :total_ingress_stats),
  total_outgress_stats = add_metric_map(total_outgress_stats, :total_outgress_stats),
  tags = :tags,
  num_updates = num_updates + :num_updates  
