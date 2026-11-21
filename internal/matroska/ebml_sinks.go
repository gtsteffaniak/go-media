package matroska

// The methods in this file implement ebmlSink for the Info and Tracks structs in
// probe.go. Each master element stores the children it has a field for; any other
// child is decoded and checked but dropped. A repeated scalar keeps its last
// value, a repeated pointer element is replaced, and a repeated list element is
// appended.

// ebmlChild returns the Info element, which collects repeated Info elements.
func (e *infoElement) ebmlChild(id uint32) ebmlSink {
	if id == idInfo {
		return &e.Info
	}
	return nil
}

// ebmlLeaf ignores leaf elements outside Info.
func (e *infoElement) ebmlLeaf(uint32, []byte) {}

// ebmlChild returns the Tracks element, which collects repeated Tracks elements.
func (e *tracksElement) ebmlChild(id uint32) ebmlSink {
	if id == idTracks {
		return &e.Tracks
	}
	return nil
}

// ebmlLeaf ignores leaf elements outside Tracks.
func (e *tracksElement) ebmlLeaf(uint32, []byte) {}

// ebmlChild returns nil: Info has no master children that go-media keeps.
func (s *segmentInfo) ebmlChild(uint32) ebmlSink { return nil }

// ebmlLeaf stores TimecodeScale and Duration.
func (s *segmentInfo) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidTimecodeScale:
		s.TimecodeScale = uintPayload(data)
	case eidDuration:
		s.Duration = floatPayload(data)
	}
}

// ebmlChild appends a TrackEntry and returns it.
func (s *segmentTracks) ebmlChild(id uint32) ebmlSink {
	if id != eidTrackEntry {
		return nil
	}
	s.TrackEntry = append(s.TrackEntry, trackEntry{})
	return &s.TrackEntry[len(s.TrackEntry)-1]
}

// ebmlLeaf ignores leaf elements directly inside Tracks.
func (s *segmentTracks) ebmlLeaf(uint32, []byte) {}

// ebmlChild replaces the Video, Audio or ContentEncodings element and returns it.
func (t *trackEntry) ebmlChild(id uint32) ebmlSink {
	switch id {
	case eidVideo:
		t.Video = &trackVideo{}
		return t.Video
	case eidAudio:
		t.Audio = &trackAudio{}
		return t.Audio
	case eidContentEncodings:
		t.ContentEncodings = &contentEncodings{}
		return t.ContentEncodings
	}
	return nil
}

// ebmlLeaf stores the TrackEntry fields go-media uses.
func (t *trackEntry) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidTrackNumber:
		t.TrackNumber = uintPayload(data)
	case eidTrackType:
		t.TrackType = uintPayload(data)
	case eidCodecID:
		t.CodecID = stringPayload(data)
	case eidCodecPrivate:
		t.CodecPrivate = data
	case eidLanguage:
		t.Language = stringPayload(data)
	case eidName:
		t.Name = stringPayload(data)
	case eidDefaultDuration:
		t.DefaultDuration = uintPayload(data)
	case eidCodecDelay:
		t.CodecDelay = uintPayload(data)
	case eidTrackTimestampScale:
		t.TrackTimestampScale = floatPayload(data)
	}
}

// ebmlChild returns nil: Video has no master children that go-media keeps.
func (v *trackVideo) ebmlChild(uint32) ebmlSink { return nil }

// ebmlLeaf stores PixelWidth and PixelHeight.
func (v *trackVideo) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidPixelWidth:
		v.PixelWidth = uintPayload(data)
	case eidPixelHeight:
		v.PixelHeight = uintPayload(data)
	}
}

// ebmlChild returns nil: Audio has no master children that go-media keeps.
func (a *trackAudio) ebmlChild(uint32) ebmlSink { return nil }

// ebmlLeaf stores SamplingFrequency, Channels and BitDepth.
func (a *trackAudio) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidSamplingFrequency:
		a.SamplingFrequency = floatPayload(data)
	case eidChannels:
		a.Channels = uintPayload(data)
	case eidBitDepth:
		a.BitDepth = uintPayload(data)
	}
}

// ebmlChild appends a ContentEncoding and returns it.
func (c *contentEncodings) ebmlChild(id uint32) ebmlSink {
	if id != eidContentEncoding {
		return nil
	}
	c.ContentEncoding = append(c.ContentEncoding, contentEncoding{})
	return &c.ContentEncoding[len(c.ContentEncoding)-1]
}

// ebmlLeaf ignores leaf elements directly inside ContentEncodings.
func (c *contentEncodings) ebmlLeaf(uint32, []byte) {}

// ebmlChild replaces the ContentCompression element and returns it.
func (c *contentEncoding) ebmlChild(id uint32) ebmlSink {
	if id != eidContentCompression {
		return nil
	}
	c.ContentCompression = &contentCompression{}
	return c.ContentCompression
}

// ebmlLeaf stores ContentEncodingScope and ContentEncodingType.
func (c *contentEncoding) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidContentEncodingScope:
		c.ContentEncodingScope = uintPayload(data)
	case eidContentEncodingType:
		c.ContentEncodingType = uintPayload(data)
	}
}

// ebmlChild returns nil: ContentCompression has no master children.
func (c *contentCompression) ebmlChild(uint32) ebmlSink { return nil }

// ebmlLeaf stores ContentCompAlgo and ContentCompSettings.
func (c *contentCompression) ebmlLeaf(id uint32, data []byte) {
	switch id {
	case eidContentCompAlgo:
		c.ContentCompAlgo = uintPayload(data)
	case eidContentCompSettings:
		c.ContentCompSettings = data
	}
}
