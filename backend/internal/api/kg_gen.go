// REQ-355 R2/M155 装配层闭包：chat.GenerateStructured/GenerateText → kg.GenFuncs。
// kg 包保持零 chat 依赖（摘除不再拖出对话包）——沿 kb.SetWikiLLM「装配层闭包防环」同款先例，
// 生成语义（连接解析/JSON 提取/重试）单源留在 chat.GenerateStructured，本文件只做签名转接。
package api

import (
	"context"
	"encoding/json"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/kg"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// ChatGenFuncs 构造 kg 生成注入（DraftJSON 投影=chat.GenerateStructured 的模型输出原文）。
func ChatGenFuncs(st *store.Store, box *secrets.Box) kg.GenFuncs {
	return kg.GenFuncs{
		GenStructured: func(ctx context.Context, connID, prompt, schemaJSON string) (json.RawMessage, error) {
			res, err := chat.GenerateStructured(ctx, st, box, connID, prompt, schemaJSON)
			if err != nil {
				return nil, err
			}
			return res.DraftJSON, nil
		},
		GenText: func(ctx context.Context, connID, system, user string, temperature *float64) (string, error) {
			return chat.GenerateText(ctx, st, box, connID, system, user, temperature)
		},
	}
}
