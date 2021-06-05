package cli

import (
	"encoding/json"
	"io"
)

func Add(id, path string, x, y int) AddCmd {
	return AddCmd{
		ID:     id,
		X:      x,
		Y:      y,
		Path:   path,
		Draw:   true,
		Scaler: Contain,
	}
}

func Remove(id string) RemoveCmd {
	return RemoveCmd{ID: id, Draw: true}
}

type Command interface{ JSON(io.Writer) error }

type AddCmd struct {
	ID                string  `json:"identifier"`
	X                 int     `json:"x"`
	Y                 int     `json:"y"`
	Path              string  `json:"path"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	Draw              bool    `json:"draw"`
	SynchronouslyDraw bool    `json:"synchronously_draw"`
	Scaler            Scaler  `json:"scaler"`
	ScalingPositionX  float64 `json:"scaling_position_x"`
	ScalingPositionY  float64 `json:"scaling_position_y"`

	Action string `json:"action"`
}

func (a AddCmd) JSON(w io.Writer) error {
	a.Action = "add"
	return json.NewEncoder(w).Encode(a)
}

type RemoveCmd struct {
	ID   string `json:"identifier"`
	Draw bool   `json:"draw"`

	Action string `json:"action"`
}

func (r RemoveCmd) JSON(w io.Writer) error {
	r.Action = "remove"
	return json.NewEncoder(w).Encode(r)
}

type Scaler string

const (
	Crop        Scaler = "crop"
	Distort     Scaler = "distort"
	FitContain  Scaler = "fit_contain"
	Contain     Scaler = "contain"
	ForcedCover Scaler = "forced_cover"
	Cover       Scaler = "cover"
)
