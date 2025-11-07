package service

import (
	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"

	"github.com/young2j/gocopy"
)

type TagService struct {
	tagDao *dao.TagDao
}

func NewTagService(tagDao *dao.TagDao) *TagService {
	return &TagService{
		tagDao: tagDao,
	}
}

var subTypeMap = map[string]string{
	"multimodal": "Multimodal",
	"cv":         "Computer Vision",
	"nlp":        "Natural Language Processing",
	"audio":      "Audio",
	"tabular":    "Tabular",
	"rl":         "Reinforcement Learning",
	"other":      "Other",
}

var orderedSubTypes = []string{
	"multimodal",
	"cv",
	"nlp",
	"audio",
	"tabular",
	"rl",
	"other",
}

var targetLabels = []string{
	"Text Generation",
	"Any-to-Any",
	"Image-Text-to-Text",
	"Image-to-Text",
	"Image-to-Image",
	"Text-to-Image",
	"Text-to-Video",
	"Text-to-Speech",
	"PyTorch",
	"TensorFlow",
	"JAX",
	"Transformers",
	"Diffusers",
	"Safetensors",
	"ONNX",
	"GGUF",
	"Transformers.js",
	"MLX",
	"Keras",
}

// type字段值映射表
var typeMapping = map[string]string{
	"pipeline_tag": "Tasks",
	"library":      "Libraries",
}

func (t *TagService) TagListByCondition(query *query.TagQuery) ([]*dto.Tag, error) {
	tags, err := t.tagDao.TagListByCondition(query)
	if err != nil {
		return nil, err
	}

	if query.DataType == "datasets" && query.Types[0] != "language" && query.Types[0] != "license" {
		for _, tag := range tags {
			tag.Type = tag.Type[5:]
		}
	}

	tagDTOs := make([]*dto.Tag, 0, len(tags))
	gocopy.Copy(&tagDTOs, &tags)

	return tagDTOs, nil
}

func (t *TagService) TaskTagList(query *query.TagQuery) ([]*dto.GroupedTagDTO, error) {
	tags, err := t.tagDao.TagListByCondition(query)
	if err != nil {
		return nil, err
	}

	tagDTOs := make([]*dto.Tag, 0, len(tags))
	gocopy.Copy(&tagDTOs, &tags)

	groupMap := make(map[string][]*dto.Tag)
	for _, tag := range tagDTOs {
		groupMap[tag.SubType] = append(groupMap[tag.SubType], tag)
	}

	result := make([]*dto.GroupedTagDTO, 0)
	for _, subType := range orderedSubTypes {
		if tagsInGroup, exists := groupMap[subType]; exists {
			result = append(result, &dto.GroupedTagDTO{
				SubType: subTypeMap[subType],
				Tags:    tagsInGroup,
			})
		}
	}

	return result, nil
}

func (t *TagService) MainTagList(datasetStr string) ([]*dto.GroupedByTypeDTO, error) {
	listCondition := &query.TagQuery{
		Labels: targetLabels,
	}
	if datasetStr == "datasets" {
		listCondition = &query.TagQuery{
			Types: []string{"Modalities", "Format"},
		}
	}

	tags, err := t.tagDao.TagListByCondition(listCondition)
	if err != nil {
		return nil, err
	}

	tagDTOs := make([]*dto.Tag, 0, len(tags))
	gocopy.Copy(&tagDTOs, &tags)

	groupMap := make(map[string][]*dto.Tag)
	for _, tag := range tagDTOs {
		groupMap[tag.Type] = append(groupMap[tag.Type], tag)
	}

	result := make([]*dto.GroupedByTypeDTO, 0, len(groupMap))
	for typ, tagsInGroup := range groupMap {
		countCondition := &query.TagQuery{
			Types: []string{typ},
		}

		totalCount, err := t.tagDao.TagCountByCondition(countCondition)
		if err != nil {
			return nil, err
		}

		mappedType, exists := typeMapping[typ]
		if !exists {
			mappedType = typ
		}

		result = append(result, &dto.GroupedByTypeDTO{
			Type:     mappedType,
			Tags:     tagsInGroup,
			TotalNum: int(totalCount),
		})
	}

	return result, nil
}
