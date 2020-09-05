package funcs

import "testing"

func TestCheckName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "good_func_name_1", args: args{"_"}},
		{name: "good_func_name_2", args: args{"a"}},
		{name: "good_func_name_3", args: args{"a1"}},
		{name: "good_func_name_4", args: args{"a1"}},
		{name: "good_func_name_5", args: args{"Ӵ"}},
		{name: "bad_func_name_1", args: args{""}, wantErr: true},
		{name: "bad_func_name_2", args: args{"2"}, wantErr: true},
		{name: "bad_func_name_3", args: args{"a-b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckName(tt.args.name); (err != nil) != tt.wantErr {
				t.Errorf("CheckName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
