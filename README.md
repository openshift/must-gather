must-gather
===========

`openshift-must-gather` is a tool for collecting cluster data.
It dumps `clusteroperator` data, and associated namespace data, into a specified `--base-dir` location.
The directory structure, as well as specific details behind this tool can be found [in this doc](https://docs.google.com/document/d/1v975fm3bjVzTPmtWYW0TQB05P3PPWUgG4AFGjD8lmOU/edit#heading=h.xbqgzreoju2s).

### Building

Place in GOPATH under `src/github.com/openshift/must-gather`.

Build with:
```
$ make
```

**Before Running**: Make sure you have a `KUBECONFIG` environment variable set, and that it points to a valid admin kubeconfig file.
Alternatively, provide a filepath to a valid `admin.kubeconfig` file via the `--kubeconfig` flag.

Run with:
```
./bin/openshift-must-gather inspect clusteroperator/<name>
```

Note: it is possible to run this tool to collect all clusteroperator data by omitting a `<name>`:

```
./bin/openshift-must-gather inspect clusteroperators
```
