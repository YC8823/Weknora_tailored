package chatpipeline

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/chunker"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginVisionAugment injects knowledge-base images into chatManage.Images so
// that the downstream multimodal LLM receives visual context alongside the
// retrieved text chunks.
//
// Two kinds of image references in merged chunks are handled:
//
//  1. Relative paths (e.g. figures/xxx.png) — from markdown-format knowledge:
//     The plugin resolves the owning knowledge entry's file_path
//     (local://tenant/id/ts.md) and prepends its directory prefix to obtain
//     the absolute local:// URL (e.g. local://10000/<id>/figures/xxx.png).
//
//  2. Absolute local:// URLs — from PPT / base64-embedded markdown ingestion:
//     image_resolver already stored the image and replaced the reference with a
//     provider:// URL during ingestion. These are added directly without any
//     knowledge directory lookup.
//
// All collected URLs are appended to chatManage.Images. resolveImageURLForLLM
// reads them from storage and converts them to base64 data URIs for the LLM.
//
// The plugin is a no-op when:
//   - The selected chat model does not support vision.
//   - No merged chunks contain image references.
type PluginVisionAugment struct {
	knowledgeService interfaces.KnowledgeService
}

// NewPluginVisionAugment creates and registers PluginVisionAugment.
func NewPluginVisionAugment(eventManager *EventManager, knowledgeService interfaces.KnowledgeService) *PluginVisionAugment {
	p := &PluginVisionAugment{knowledgeService: knowledgeService}
	eventManager.Register(p)
	return p
}

// ActivationEvents returns the event type this plugin handles.
func (p *PluginVisionAugment) ActivationEvents() []types.EventType {
	return []types.EventType{types.VISION_AUGMENT}
}

// OnEvent augments chatManage.Images with images referenced by the merged chunks.
func (p *PluginVisionAugment) OnEvent(
	ctx context.Context,
	_ types.EventType,
	chatManage *types.ChatManage,
	next func() *PluginError,
) *PluginError {
	pipelineInfo(ctx, "VisionAugment", "enter", map[string]interface{}{
		"supports_vision": chatManage.ChatModelSupportsVision,
		"merge_result_cnt": len(chatManage.MergeResult),
	})

	if !chatManage.ChatModelSupportsVision {
		pipelineInfo(ctx, "VisionAugment", "skip", map[string]interface{}{"reason": "model_no_vision"})
		return next()
	}
	if len(chatManage.MergeResult) == 0 {
		pipelineInfo(ctx, "VisionAugment", "skip", map[string]interface{}{"reason": "no_merge_result"})
		return next()
	}

	// Group relative image refs by knowledge ID; collect absolute local:// refs directly.
	// http/data URIs are already handled by the existing user-image pipeline.
	knowledgeRefs := make(map[string][]string) // knowledgeID → []relPath
	var directLocalURLs []string
	for _, result := range chatManage.MergeResult {
		refs := chunker.ExtractImageRefs(result.Content)
		for _, ref := range refs {
			url := ref.OriginalRef
			if strings.HasPrefix(url, "http://") ||
				strings.HasPrefix(url, "https://") ||
				strings.HasPrefix(url, "data:") {
				continue
			}
			if strings.HasPrefix(url, "local://") {
				// Already an absolute provider URL (e.g. from PPT or base64-MD ingestion).
				// Add directly without needing a knowledge directory lookup.
				directLocalURLs = append(directLocalURLs, url)
				continue
			}
			knowledgeRefs[result.KnowledgeID] = append(knowledgeRefs[result.KnowledgeID], url)
		}
	}

	if len(knowledgeRefs) == 0 && len(directLocalURLs) == 0 {
		pipelineInfo(ctx, "VisionAugment", "skip", map[string]interface{}{
			"reason":    "no_image_refs_in_chunks",
			"chunk_cnt": len(chatManage.MergeResult),
		})
		return next()
	}

	// Batch-fetch the file_path for each knowledge entry (only needed for relative refs).
	dirPrefix := make(map[string]string)
	if len(knowledgeRefs) > 0 {
		knowledgeIDs := make([]string, 0, len(knowledgeRefs))
		for id := range knowledgeRefs {
			knowledgeIDs = append(knowledgeIDs, id)
		}

		tenantID, _ := types.TenantIDFromContext(ctx)
		if tenantID == 0 {
			tenantID = chatManage.TenantID
		}

		knowledgeList, err := p.knowledgeService.GetKnowledgeBatch(ctx, tenantID, knowledgeIDs)
		if err != nil {
			pipelineWarn(ctx, "VisionAugment", "fetch_knowledge_failed", map[string]interface{}{
				"error": err.Error(),
			})
			return next()
		}

		// Build knowledgeID → directory prefix map.
		// e.g. "local://10000/75db1990-.../1779936643147786670.md"
		//   →  "local://10000/75db1990-.../"
		dirPrefix = make(map[string]string, len(knowledgeList))
		for _, k := range knowledgeList {
			if k == nil || k.FilePath == "" {
				continue
			}
			idx := strings.LastIndex(k.FilePath, "/")
			if idx < 0 {
				continue
			}
			dirPrefix[k.ID] = k.FilePath[:idx+1]
		}
	}

	// Append unique local:// image URLs to chatManage.Images.
	seen := make(map[string]bool, len(chatManage.Images))
	for _, img := range chatManage.Images {
		seen[img] = true
	}

	added := 0
	for knowledgeID, relPaths := range knowledgeRefs {
		prefix, ok := dirPrefix[knowledgeID]
		if !ok {
			continue
		}
		for _, rel := range relPaths {
			localURL := prefix + rel
			if !seen[localURL] {
				seen[localURL] = true
				chatManage.Images = append(chatManage.Images, localURL)
				added++
			}
		}
	}
	for _, url := range directLocalURLs {
		if !seen[url] {
			seen[url] = true
			chatManage.Images = append(chatManage.Images, url)
			added++
		}
	}

	pipelineInfo(ctx, "VisionAugment", "images_augmented", map[string]interface{}{
		"added":        added,
		"total_images": len(chatManage.Images),
	})

	return next()
}
