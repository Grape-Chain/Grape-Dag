package tx

import (
	"bytes"
	"fmt"

	pb "github.com/VG-Grape/luna/tx/pb"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

type Ids struct {
	IDs        []UuidSlice
	Signatures map[string][]byte
}

type UuidSlice struct {
	Is_empty bool
	Id       uuid.UUID
}

func (_ids_ *Ids) MarshalBinary() *pb.Ids {
	pbIds := &pb.Ids{
		Ids:        []*pb.UuidSlice{},
		Signatures: _ids_.Signatures,
	}
	goterators.ForEach(_ids_.IDs, func(slice UuidSlice) {
		pbIds.Ids = append(pbIds.Ids, &pb.UuidSlice{
			IsEmpty: slice.Is_empty,
			Uuid:    slice.Id[:],
		})
	})
	return pbIds
}

func (_ids_ *Ids) UnmarshalBinary(pbIds *pb.Ids) {
	_ids_.Signatures = pbIds.Signatures
	goterators.ForEach(pbIds.Ids, func(pbid *pb.UuidSlice) {
		var _uuid_ uuid.UUID
		var empty = pbid.IsEmpty
		var err error
		if !empty {
			_uuid_, err = uuid.FromBytes(pbid.Uuid)
			if err != nil {
				_uuid_ = uuid.Nil
				empty = true
			}
		} else {
			_uuid_ = uuid.Nil
		}
		_ids_.IDs = append(_ids_.IDs, UuidSlice{
			Is_empty: empty,
			Id:       _uuid_,
		})
	})
}

func (ids *Ids) String() string {
	strBuf := bytes.Buffer{}
	strBuf.WriteString(fmt.Sprint(ids.Signatures))
	goterators.ForEach(ids.IDs, func(id UuidSlice) {
		strBuf.WriteString(id.Id.String() + " ")
	})
	return strBuf.String()
}
