"""add api_key to users

Revision ID: 004_add_api_key
Revises: 003_add_missing_tables
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa

revision: str = '004_add_api_key'
down_revision: Union[str, None] = '003_add_missing_tables'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    with op.batch_alter_table('users') as batch_op:
        batch_op.add_column(sa.Column('api_key', sa.String(length=64), nullable=True))
        batch_op.create_unique_constraint('uq_users_api_key', ['api_key'])


def downgrade() -> None:
    with op.batch_alter_table('users') as batch_op:
        batch_op.drop_constraint('uq_users_api_key', type_='unique')
        batch_op.drop_column('api_key')
