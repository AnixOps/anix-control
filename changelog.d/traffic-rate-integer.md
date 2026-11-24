### Fixed

- A node's rate is applied to reported traffic in integer arithmetic: the
  rate to 1/1000 and the result rounded down to the byte, exactly for
  every 64-bit counter (floating point lost the last bytes of counters
  above 2^53). A user's or batch's total that would pass the 64-bit
  counters is refused with `report_counter_overflow` instead of wrapping
  to a negative number. A rate that is not a positive number counts as 1.
