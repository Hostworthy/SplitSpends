package routes

import (
	"net/http"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

func toCategory(c *category.Category, viewer uuid.UUID) platform.CategoryEntry {
	return platform.CategoryEntry{
		Category: platform.Category{ID: c.ID, Name: c.Name, Group: c.Group, Icon: c.Icon},
		Custom:   c.Custom(),
		Mine:     c.Custom() && c.OwnerID == viewer,
	}
}

func toCategoryInput(in platform.CategoryInput) category.Input {
	return category.Input{ID: in.ID, Name: in.Name, Icon: in.Icon, Group: in.Group}
}

func (a *API) listCategories(w http.ResponseWriter, r *http.Request) error {
	all, err := a.svc.Categories.List(r.Context(), me(r))
	if err != nil {
		return err
	}
	out := make([]platform.CategoryEntry, 0, len(all))
	for _, c := range all {
		out = append(out, toCategory(c, me(r)))
	}
	return paged(w, r, out)
}

func (a *API) createCategory(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.CategoryInput](r)
	if err != nil {
		return err
	}
	c, err := a.svc.Categories.Create(r.Context(), me(r), toCategoryInput(in))
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toCategory(c, me(r)))
}

func (a *API) patchCategory(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.CategoryInput](r)
	if err != nil {
		return err
	}
	c, err := a.svc.Categories.Update(r.Context(), me(r), r.PathValue("categoryId"), toCategoryInput(in))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toCategory(c, me(r)))
}

func (a *API) deleteCategory(w http.ResponseWriter, r *http.Request) error {
	if err := a.svc.Categories.Delete(r.Context(), me(r), r.PathValue("categoryId")); err != nil {
		return err
	}
	return noContent(w)
}
