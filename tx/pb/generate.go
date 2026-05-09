//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative txvX.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative node.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative ids.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative txpin.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative edge.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative vertex.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative lunatx.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative sync.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative secret.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative txgen.proto
//go:generate protoc -I=. --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative vm.proto
package pb
