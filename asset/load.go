package asset

import (
	"errors"
	"io/fs"
	"image"
	_ "image/png"
	"encoding/json"
	"io/ioutil"
	"path"

	"github.com/faiface/pixel"
	"github.com/jstewart7/packer"
)

type Load struct {
	filesystem fs.FS
}

func NewLoad(filesystem fs.FS) *Load {
	return &Load{filesystem}
}

func (load *Load) Open(filepath string) (fs.File, error) {
	return load.filesystem.Open(filepath)
}

func (load *Load) Image(filepath string) (image.Image, error) {
	file, err := load.filesystem.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (load *Load) Sprite(filepath string) (*pixel.Sprite, error) {
	img, err := load.Image(filepath)
	if err != nil {
		return nil, err
	}
	pic := pixel.PictureDataFromImage(img)
	return pixel.NewSprite(pic, pic.Bounds()), nil
}

func (load *Load) Json(filepath string, dat interface{}) error {
	file, err := load.filesystem.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonData, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonData, dat)
}

func (load *Load) Spritesheet(filepath string) (*Spritesheet, error) {
	//Load the Json
	serializedSpritesheet := packer.SerializedSpritesheet{}
	err := load.Json(filepath, &serializedSpritesheet)
	if err != nil {
		return nil, err
	}

	imageFilepath := path.Join(path.Dir(filepath), serializedSpritesheet.ImageName)

	// Load the image
	img, err := load.Image(imageFilepath)
	if err != nil {
		return nil, err
	}
	pic := pixel.PictureDataFromImage(img)

	// Create the spritesheet object
	bounds := pic.Bounds()
	lookup := make(map[string]*pixel.Sprite)
	for k, v := range serializedSpritesheet.Frames {
		rect := pixel.R(
			v.Frame.X,
			bounds.H() - v.Frame.Y,
			v.Frame.X + v.Frame.W,
			bounds.H() - (v.Frame.Y + v.Frame.H)).Norm()

		lookup[k] = pixel.NewSprite(pic, rect)
	}

	return NewSpritesheet(pic, lookup), nil
}

type Spritesheet struct {
	picture pixel.Picture
	lookup map[string]*pixel.Sprite
}

func NewSpritesheet(pic pixel.Picture, lookup map[string]*pixel.Sprite) *Spritesheet {
	return &Spritesheet{
		picture: pic,
		lookup: lookup,
	}
}

func (s *Spritesheet) Get(name string) (*pixel.Sprite, error) {
	sprite, ok := s.lookup[name]
	if !ok {
		return nil, errors.New("Invalid sprite name!")
	}
	return sprite, nil
}

func (s *Spritesheet) Picture() pixel.Picture {
	return s.picture
}
