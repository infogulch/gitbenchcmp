# gitbenchcmp
benchcmp that works with git refs

## Example
```
csv>git branch
  csv-addbench
  csv-reduceallocs
* csv-streamreader
  master

csv>gitbenchcmp -test.benchmem csv-addbench csv-reduceallocs

benchmark                 old ns/op     new ns/op     delta
BenchmarkRead-4           7844          6599          -15.87%
BenchmarkReadNLarge-4     2722          1500          -44.89%
BenchmarkReadNSmall-4     376           270           -28.19%

benchmark                 old allocs     new allocs     delta
BenchmarkRead-4           41             30             -26.83%
BenchmarkReadNLarge-4     27             2              -92.59%
BenchmarkReadNSmall-4     4              2              -50.00%

benchmark                 old bytes     new bytes     delta
BenchmarkRead-4           5844          5704          -2.40%
BenchmarkReadNLarge-4     442           448           +1.36%
BenchmarkReadNSmall-4     51            51            +0.00%

csv>git branch
  csv-addbench
  csv-reduceallocs
* csv-streamreader 
```

## Usage
```
Usage: gitbenchcmp [flags] <commit-ish>...
Flags:
  -best
        compare best times
  -changed
        show only benchmarks that have changed
  -mag
        sort benchmarks by magnitude of change
  -outdir string
        directory to store benchmark results. if blank, uses the OS's temp dir and is cleaned up afterwards
  -test.bench string
        run benchmarks matching the regular expression (default ".")
  -test.benchmem
        include memory allocation statistics for comparison
  -test.run string
        run only the tests and examples matching the regular expression (default "NONE")
  -test.short
        tell long running tests to shorten their run time
  -verbose
        chatty logging
```
