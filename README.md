must-gather
===========

`openshift-must-gather` is a tool for collecting cluster data.
It dumps `clusteroperator` data, and associated namespace data, into a specified `--base-dir` location.
The directory structure, as well as specific details behind this tool can be found [in this doc](https://docs.google.com/document/d/1v975fm3bjVzTPmtWYW0TQB05P3PPWUgG4AFGjD8lmOU/edit#heading=h.xbqgzreoju2s).

## Collection Scripts
Data collection scripts are kept in `./collection-scripts`.  The content of that folder is placed in `/usr/bin` in the image.
The data collection scripts should only include collection logic for components that are included as part of the OpenShift
CVO payload.  Outside components are encouraged to produce a similar "must-gather" image, but this is not the spot to be
included. 

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

#### Developers Only

**WARNING**: The following tool is provided with no guarantees and might (and will) be changed at any time. Please do not rely on anything below in your scripts or automatization.

Beside the `oppenshift-must-gather` binary the `openshift-dev-helpers` binary is also provided. This binary combines various
tools useful for the OpenShift developers teams. 

To list all events recorded during a test run and stored in `events.json` file, you can run this command:
```bash
./bin/openshift-dev-helpers events https://storage.googleapis.com/origin-ci-test/pr-logs/.../artifacts/e2e-aws/events.json --component=openshift-apiserver-operator
```
