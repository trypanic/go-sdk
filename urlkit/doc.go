// Package urlkit builds URLs and joins paths safely: BuildURL appends a path
// and query parameters to a base URL without letting the path change its
// scheme, host, query or fragment, and JoinPath/JoinHTTPPath join path
// segments while rejecting "." and ".." segments. Failures are
// *errorkit.AppError values with validation codes.
package urlkit
