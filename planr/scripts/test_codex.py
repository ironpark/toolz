from __future__ import annotations

import unittest

from codex import (
    DEFAULT_REASONING,
    final_response_from_items,
    make_prompts,
    token_usage_summary,
)


class HarnessHelpersTest(unittest.TestCase):
    def test_token_usage_prefers_last_turn_breakdown(self) -> None:
        usage = token_usage_summary(
            {
                "last": {
                    "inputTokens": 11,
                    "cachedInputTokens": 3,
                    "outputTokens": 7,
                    "reasoningOutputTokens": 2,
                    "totalTokens": 18,
                },
                "total": {
                    "inputTokens": 99,
                    "cachedInputTokens": 50,
                    "outputTokens": 40,
                    "reasoningOutputTokens": 10,
                    "totalTokens": 139,
                },
            }
        )
        self.assertEqual(
            usage,
            {
                "input_tokens": 11,
                "cached_input_tokens": 3,
                "output_tokens": 7,
                "reasoning_output_tokens": 2,
                "total_tokens": 18,
            },
        )

    def test_final_response_uses_last_agent_message(self) -> None:
        items = [
            {"type": "agentMessage", "text": "first"},
            {"type": "commandExecution", "command": "go test ./..."},
            {"type": "agentMessage", "text": "last"},
        ]
        self.assertEqual(final_response_from_items(items), "last")

    def test_prompt_sequence_has_final_verification(self) -> None:
        prompts = make_prompts(3)
        self.assertEqual(len(prompts), 3)
        self.assertIn("planr new", prompts[0])
        self.assertIn("Final verification", prompts[-1])
        self.assertEqual(DEFAULT_REASONING, "medium")


if __name__ == "__main__":
    unittest.main()
