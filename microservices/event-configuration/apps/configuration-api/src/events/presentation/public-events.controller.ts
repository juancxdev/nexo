import { Controller, Get, Query } from '@nestjs/common';
import { ApiOperation, ApiResponse, ApiTags } from '@nestjs/swagger';
import { DatabaseService } from '../../shared/database/database.service';

@ApiTags('Public Events')
@Controller('public/events')
export class PublicEventsController {
  constructor(private readonly databaseService: DatabaseService) {}

  @Get()
  @ApiOperation({ summary: 'List published events for the public Nexo landing carousel.' })
  @ApiResponse({ status: 200, description: 'Published events list.' })
  async listPublished(@Query('limit') limit?: string) {
    const parsed = limit === undefined ? 20 : Number(limit);
    const safeLimit = Number.isInteger(parsed) && parsed > 0 ? Math.min(parsed, 50) : 20;
    const items = await this.databaseService.listPublicPublishedEvents(safeLimit);
    return { items };
  }
}
