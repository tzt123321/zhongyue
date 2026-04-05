"""add reverb params to effect_presets

Revision ID: 9622447fbb88
Revises: e95f7d27b7ec
Create Date: 2026-03-27 11:48:20.631415

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '9622447fbb88'
down_revision: Union[str, None] = 'e95f7d27b7ec'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('effect_presets', sa.Column('dry_wet', sa.Float(), nullable=True))
    op.add_column('effect_presets', sa.Column('room_size', sa.Float(), nullable=True))
    op.add_column('effect_presets', sa.Column('damping', sa.Float(), nullable=True))
    op.add_column('effect_presets', sa.Column('stereo_width', sa.Float(), nullable=True))


def downgrade() -> None:
    op.drop_column('effect_presets', 'stereo_width')
    op.drop_column('effect_presets', 'damping')
    op.drop_column('effect_presets', 'room_size')
    op.drop_column('effect_presets', 'dry_wet')
