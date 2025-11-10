"""
Архитектура нейросети Allan - GPT-like модель для русского языка.
Создана с нуля специально для обработки русскоязычных текстов.
"""

import math
import torch
import torch.nn as nn
import torch.nn.functional as F
from dataclasses import dataclass
from typing import Optional, Tuple


@dataclass
class AllanConfig:
    """Конфигурация модели Allan."""

    # Размерности модели
    vocab_size: int = 50000  # Размер словаря
    n_embd: int = 768  # Размерность эмбеддингов
    n_layer: int = 12  # Количество слоев трансформера
    n_head: int = 12  # Количество голов внимания
    n_positions: int = 2048  # Максимальная длина контекста

    # Regularization
    dropout: float = 0.1
    attn_dropout: float = 0.1
    embd_pdrop: float = 0.1
    resid_pdrop: float = 0.1

    # Activation
    activation: str = "gelu"

    # Layer norm
    layer_norm_epsilon: float = 1e-5

    # Initialization
    initializer_range: float = 0.02

    def __post_init__(self):
        """Валидация конфигурации."""
        assert self.n_embd % self.n_head == 0, \
            f"n_embd ({self.n_embd}) должно делиться на n_head ({self.n_head})"


class AllanAttention(nn.Module):
    """Multi-head self-attention для Allan."""

    def __init__(self, config: AllanConfig):
        super().__init__()
        self.n_head = config.n_head
        self.n_embd = config.n_embd
        self.dropout = config.attn_dropout

        assert config.n_embd % config.n_head == 0

        # Key, Query, Value проекции
        self.c_attn = nn.Linear(config.n_embd, 3 * config.n_embd)
        # Выходная проекция
        self.c_proj = nn.Linear(config.n_embd, config.n_embd)

        # Dropout
        self.attn_dropout = nn.Dropout(config.attn_dropout)
        self.resid_dropout = nn.Dropout(config.resid_pdrop)

        # Causal mask
        self.register_buffer(
            "bias",
            torch.tril(torch.ones(config.n_positions, config.n_positions))
            .view(1, 1, config.n_positions, config.n_positions)
        )

    def forward(
        self,
        x: torch.Tensor,
        attention_mask: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        B, T, C = x.size()  # batch, sequence length, embedding dimensionality

        # Вычисляем query, key, value
        q, k, v = self.c_attn(x).split(self.n_embd, dim=2)

        # Разделяем на головы
        k = k.view(B, T, self.n_head, C // self.n_head).transpose(1, 2)  # (B, nh, T, hs)
        q = q.view(B, T, self.n_head, C // self.n_head).transpose(1, 2)  # (B, nh, T, hs)
        v = v.view(B, T, self.n_head, C // self.n_head).transpose(1, 2)  # (B, nh, T, hs)

        # Вычисляем attention scores
        att = (q @ k.transpose(-2, -1)) * (1.0 / math.sqrt(k.size(-1)))

        # Применяем causal mask
        att = att.masked_fill(self.bias[:, :, :T, :T] == 0, float('-inf'))

        # Применяем attention mask если есть
        if attention_mask is not None:
            att = att.masked_fill(attention_mask == 0, float('-inf'))

        # Softmax и dropout
        att = F.softmax(att, dim=-1)
        att = self.attn_dropout(att)

        # Применяем attention к values
        y = att @ v  # (B, nh, T, hs)
        y = y.transpose(1, 2).contiguous().view(B, T, C)  # (B, T, C)

        # Выходная проекция и dropout
        y = self.resid_dropout(self.c_proj(y))

        return y


class AllanMLP(nn.Module):
    """Feed-forward сеть для Allan."""

    def __init__(self, config: AllanConfig):
        super().__init__()
        self.c_fc = nn.Linear(config.n_embd, 4 * config.n_embd)
        self.c_proj = nn.Linear(4 * config.n_embd, config.n_embd)
        self.act = nn.GELU() if config.activation == "gelu" else nn.ReLU()
        self.dropout = nn.Dropout(config.resid_pdrop)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        x = self.c_fc(x)
        x = self.act(x)
        x = self.c_proj(x)
        x = self.dropout(x)
        return x


class AllanBlock(nn.Module):
    """Transformer блок для Allan."""

    def __init__(self, config: AllanConfig):
        super().__init__()
        self.ln_1 = nn.LayerNorm(config.n_embd, eps=config.layer_norm_epsilon)
        self.attn = AllanAttention(config)
        self.ln_2 = nn.LayerNorm(config.n_embd, eps=config.layer_norm_epsilon)
        self.mlp = AllanMLP(config)

    def forward(
        self,
        x: torch.Tensor,
        attention_mask: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        # Pre-norm architecture
        x = x + self.attn(self.ln_1(x), attention_mask=attention_mask)
        x = x + self.mlp(self.ln_2(x))
        return x


class AllanGPT(nn.Module):
    """
    Allan GPT - Языковая модель для русского языка.
    Архитектура похожа на GPT, но оптимизирована для русскоязычных текстов.
    """

    def __init__(self, config: AllanConfig):
        super().__init__()
        self.config = config

        # Эмбеддинги
        self.wte = nn.Embedding(config.vocab_size, config.n_embd)  # token embeddings
        self.wpe = nn.Embedding(config.n_positions, config.n_embd)  # position embeddings
        self.drop = nn.Dropout(config.embd_pdrop)

        # Transformer блоки
        self.h = nn.ModuleList([AllanBlock(config) for _ in range(config.n_layer)])

        # Финальная нормализация
        self.ln_f = nn.LayerNorm(config.n_embd, eps=config.layer_norm_epsilon)

        # Language modeling head
        self.lm_head = nn.Linear(config.n_embd, config.vocab_size, bias=False)

        # Инициализация весов
        self.apply(self._init_weights)

        # Связываем веса эмбеддингов и головы
        self.lm_head.weight = self.wte.weight

        # Считаем количество параметров
        n_params = sum(p.numel() for p in self.parameters())
        print(f"Allan GPT создан с {n_params/1e6:.1f}M параметров")

    def _init_weights(self, module):
        """Инициализация весов."""
        if isinstance(module, nn.Linear):
            torch.nn.init.normal_(module.weight, mean=0.0, std=self.config.initializer_range)
            if module.bias is not None:
                torch.nn.init.zeros_(module.bias)
        elif isinstance(module, nn.Embedding):
            torch.nn.init.normal_(module.weight, mean=0.0, std=self.config.initializer_range)
        elif isinstance(module, nn.LayerNorm):
            torch.nn.init.zeros_(module.bias)
            torch.nn.init.ones_(module.weight)

    def forward(
        self,
        input_ids: torch.Tensor,
        attention_mask: Optional[torch.Tensor] = None,
        labels: Optional[torch.Tensor] = None,
    ) -> Tuple[torch.Tensor, Optional[torch.Tensor]]:
        """
        Forward pass.

        Args:
            input_ids: (batch_size, seq_len)
            attention_mask: (batch_size, seq_len)
            labels: (batch_size, seq_len) для вычисления loss

        Returns:
            logits: (batch_size, seq_len, vocab_size)
            loss: scalar если labels предоставлены
        """
        device = input_ids.device
        b, t = input_ids.size()
        assert t <= self.config.n_positions, \
            f"Последовательность длины {t} превышает максимум {self.config.n_positions}"

        # Позиционные индексы
        pos = torch.arange(0, t, dtype=torch.long, device=device).unsqueeze(0)  # (1, t)

        # Эмбеддинги
        tok_emb = self.wte(input_ids)  # (b, t, n_embd)
        pos_emb = self.wpe(pos)  # (1, t, n_embd)
        x = self.drop(tok_emb + pos_emb)

        # Transformer блоки
        for block in self.h:
            x = block(x, attention_mask=attention_mask)

        # Финальная нормализация
        x = self.ln_f(x)

        # Language modeling head
        logits = self.lm_head(x)  # (b, t, vocab_size)

        # Вычисляем loss если labels предоставлены
        loss = None
        if labels is not None:
            # Сдвигаем logits и labels для causal LM
            shift_logits = logits[..., :-1, :].contiguous()
            shift_labels = labels[..., 1:].contiguous()

            # Flatten для вычисления loss
            loss = F.cross_entropy(
                shift_logits.view(-1, shift_logits.size(-1)),
                shift_labels.view(-1),
                ignore_index=-100
            )

        return logits, loss

    @torch.no_grad()
    def generate(
        self,
        input_ids: torch.Tensor,
        max_new_tokens: int = 100,
        temperature: float = 1.0,
        top_k: Optional[int] = None,
        top_p: Optional[float] = None,
    ) -> torch.Tensor:
        """
        Генерация текста.

        Args:
            input_ids: (batch_size, seq_len) начальная последовательность
            max_new_tokens: количество токенов для генерации
            temperature: температура сэмплирования
            top_k: top-k фильтрация
            top_p: nucleus (top-p) фильтрация

        Returns:
            generated: (batch_size, seq_len + max_new_tokens)
        """
        for _ in range(max_new_tokens):
            # Обрезаем если превышает максимальный контекст
            input_ids_cond = input_ids if input_ids.size(1) <= self.config.n_positions \
                else input_ids[:, -self.config.n_positions:]

            # Forward pass
            logits, _ = self(input_ids_cond)

            # Берем логиты последнего токена
            logits = logits[:, -1, :] / temperature

            # Top-k фильтрация
            if top_k is not None:
                v, _ = torch.topk(logits, min(top_k, logits.size(-1)))
                logits[logits < v[:, [-1]]] = -float('Inf')

            # Top-p (nucleus) фильтрация
            if top_p is not None:
                sorted_logits, sorted_indices = torch.sort(logits, descending=True)
                cumulative_probs = torch.cumsum(F.softmax(sorted_logits, dim=-1), dim=-1)

                # Удаляем токены с cumulative probability > top_p
                sorted_indices_to_remove = cumulative_probs > top_p
                sorted_indices_to_remove[..., 1:] = sorted_indices_to_remove[..., :-1].clone()
                sorted_indices_to_remove[..., 0] = 0

                indices_to_remove = sorted_indices_to_remove.scatter(
                    1, sorted_indices, sorted_indices_to_remove
                )
                logits[indices_to_remove] = -float('Inf')

            # Сэмплируем следующий токен
            probs = F.softmax(logits, dim=-1)
            next_token = torch.multinomial(probs, num_samples=1)

            # Добавляем к последовательности
            input_ids = torch.cat([input_ids, next_token], dim=1)

        return input_ids

    def save_pretrained(self, save_directory: str):
        """Сохранить модель."""
        import os
        import json

        os.makedirs(save_directory, exist_ok=True)

        # Сохраняем конфигурацию
        config_path = os.path.join(save_directory, "config.json")
        with open(config_path, 'w') as f:
            json.dump(self.config.__dict__, f, indent=2)

        # Сохраняем веса
        weights_path = os.path.join(save_directory, "pytorch_model.bin")
        torch.save(self.state_dict(), weights_path)

        print(f"Модель сохранена в {save_directory}")

    @classmethod
    def from_pretrained(cls, load_directory: str, device: str = 'cpu'):
        """Загрузить модель."""
        import os
        import json

        # Загружаем конфигурацию
        config_path = os.path.join(load_directory, "config.json")
        with open(config_path, 'r') as f:
            config_dict = json.load(f)
        config = AllanConfig(**config_dict)

        # Создаем модель
        model = cls(config)

        # Загружаем веса
        weights_path = os.path.join(load_directory, "pytorch_model.bin")
        state_dict = torch.load(weights_path, map_location=device)
        model.load_state_dict(state_dict)

        print(f"Модель загружена из {load_directory}")
        return model
