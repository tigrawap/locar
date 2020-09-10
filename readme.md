The goal behind locar is to make search parallel.

utilities like `find`, `du`, `ls -R` are all single threaded
This is usually not a problem for local filesystem, as whole filesystem metadata might be cached, but with remote/distributed file systems this might be a problem and paralellising of readdirs might yield huge gain in performance.

As an example:
```
>> time find trinity/ -type f | wc -l
14099914

real 15m36.272s
user 0m7.460s
sys 0m12.848s

>> time locar trinity/ --type file | wc -l
14099914

real 0m27.501s
user 1m7.076s
sys 0m39.272s
```

I.e ~almost X35 speedup


CLI still subject to change as `locar` evolves

`locar --help`
```
Usage:
  locar [OPTIONS] [directories...]

Application Options:
      --resilient    Do not stop on errors, instead print to stderr
      --inodes       Output inodes along with filenames
  -j, --jobs=        Number of jobs(threads) (default: 128)
      --with-size    Output file sizes along with filenames
      --result-jobs= Number of jobs for processing results, like doing stats to get file sizes (default: 128)
  -x, --exclude=     Patterns to exclude. Can be specified multiple times
  -f, --filter=      Patterns to filter by. Can be specified multiple times
  -t, --type=        Search entries of specific type
                     Possible values: file, dir, link, all. Can be specified multiple times (default: file, dir, link)
      --timeout=     Timeout for readdir operations. Error will be reported, but os thread will be kept hanging (default: 5m)


Help Options:
  -h, --help         Show this help message

Arguments:
  directories:       Directories to search, using current directory if missing
```
