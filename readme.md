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


Find Files Newer than 72h Using `find`
```
>> time find /scratch/img -type f -ctime -3 -mtime -3 -atime -3 | wc -l
495702

real	2m56.041s
user	0m1.390s
sys	0m8.526s
```

Find Files Newer than 72h Using `locar`

```
>> time ./locar -t file --atime-newer=72h --mtime-newer=72h --ctime-newer=72h /scratch/img | wc -l
495702

real	0m14.686s
user	0m2.349s
sys	0m4.508s
```

I.e ~almost X35 speedup


CLI still subject to change as `locar` evolves

`locar --help`
```
Usage:
  locar [OPTIONS] [directories...]

Application Options:
      --resilient      DEPRECATED and ignored, resilient is a default, use --stop-on-error if it is undesired behaviour
      --stop-on-error  Aborts scan on any error
      --inodes         Output inodes (decimal) along with filenames
      --inodes-hex     Output inodes (hexadecimal) along with filenames
      --raw            Output filenames as escaped strings
  -j, --jobs=          Number of jobs(threads) (default: 128)
      --with-size      Output file sizes along with filenames
      --with-times     Output file with atime, mtime, ctime along with filenames
      --atime-older=   Filter files by access time older than this duration (e.g., 24h5m25s) (default: 0s)
      --atime-newer=   Filter files by access time newer than this duration (e.g., 24h5m25s) (default: 0s)
      --mtime-older=   Filter files by modification time older than this duration (e.g., 24h5m25s) (default: 0s)
      --mtime-newer=   Filter files by modification time newer than this duration (e.g., 24h5m25s) (default: 0s)
      --ctime-older=   Filter files by change time older than this duration (e.g., 24h5m25s) (default: 0s)
      --ctime-newer=   Filter files by change time newer than this duration (e.g., 24h5m25s) (default: 0s)
      --result-jobs=   Number of jobs for processing results, like doing stats to get file sizes (default: 128)
      --delete         Delete found files. Non empty directories will be ignored
      --delete-all     Delete found files. Non empty directories will be removed with ALL their contents!!!
  -v, --version        Show version
  -x, --exclude=       Patterns to exclude. Can be specified multiple times
  -f, --filter=        Patterns to filter by. Can be specified multiple times
  -t, --type=          Search entries of specific type
                       Possible values: file, dir, link, socket, all. Can be specified multiple times (default: file, dir, link, socket)
      --timeout=       Timeout for readdir operations. Error will be reported, but os thread will be kept hanging (default: 5m)

Help Options:
  -h, --help           Show this help message

Arguments:
  directories:         Directories to search, using current directory if missing
```
