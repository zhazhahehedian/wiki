package registry

import "context"

// PublishedFiles deliberately uses the consumer projection even for owners;
// Read(publicOnly) already hides offline or invisible capabilities. A pinned
// version prevents importing a different release after navigation. The
// returned Detail lists only that version.
func (s *Service) PublishedFiles(ctx context.Context, user, slug, versionID string) (Detail, []File, error) {
	d, err := s.Published(ctx, user, slug)
	if err != nil {
		return Detail{}, nil, err
	}
	if d.Type != "skill" {
		return Detail{}, nil, ErrNotFound
	}
	if versionID != d.CurrentVersionID {
		return Detail{}, nil, ErrConflict
	}
	for _, v := range d.Versions {
		if v.ID == versionID && v.HasBundle {
			files, err := s.skillFiles(ctx, slug, v, true)
			d.Versions = []Version{v}
			return d, files, err
		}
	}
	return Detail{}, nil, ErrNotFound
}
